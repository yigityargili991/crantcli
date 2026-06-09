package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"crantcli/internal/cave"
	"crantcli/internal/config"
	"crantcli/internal/seatable"
	"crantcli/internal/skeleton"

	"github.com/spf13/cobra"
	"golang.org/x/term"
)

const skeletonViewerOverrideEnv = "CRANTCLI_SKELETON_VIEWER"

type skeletonViewOptions struct {
	Projection  string
	MaxNodes    int
	NoCache     bool
	JSONDebug   bool
	Wait        bool
	WaitTimeout time.Duration
}

type skeletonViewDeps struct {
	token      func() string
	lookPathUV func() (string, error)
	cacheRoot  func() (string, error)
	fetch      func(context.Context, skeleton.BridgeOptions) (*skeleton.Skeleton, error)
	viewerPath func() (string, error)
	launch     func(context.Context, string, string, string, string, int) error
	remote     func(string) skeletonRemote
	rootInfo   func(string) ([]string, error)
	rootInfoOK func() bool
	sleep      func(context.Context, time.Duration) error
}

type skeletonRemote interface {
	SkeletonExists(ctx context.Context, rootID string) (bool, error)
	QueueSkeleton(ctx context.Context, rootID string) (float64, error)
}

var skeletonViewCmd = &cobra.Command{
	Use:   "skeleton-view <root_id>",
	Short: "Open a GPU skeleton viewer for a CAVE root ID",
	Long: `Open a native GPU-accelerated viewer for a CAVE root ID skeleton.

The command uses uv to run an embedded Python bridge against CAVE skeletoncache.
Skeleton JSON is cached under ~/.cache/crantcli/skeletons.

The default view is 3D. If CAVE has not generated the skeleton yet, the
command queues generation and exits quickly unless --wait is set. The viewer
uses anti-aliased rendering with axes/grid, hover labels, PNG screenshots, and
switchable depth/branch/radius/L2 color modes. It includes compact root-info
metadata when it is available. Skeleton geometry and viewer metadata are cached
locally; --no-cache refreshes both.`,
	Args: cobra.ExactArgs(1),
}

func init() {
	var opts skeletonViewOptions
	skeletonViewCmd.Flags().StringVar(&opts.Projection, "projection", string(skeleton.Projection3D), "Initial view: 3d, xy, xz, yz, or iso")
	skeletonViewCmd.Flags().IntVar(&opts.MaxNodes, "max-nodes", 0, "Maximum nodes to render/print; 0 keeps all nodes")
	skeletonViewCmd.Flags().BoolVar(&opts.NoCache, "no-cache", false, "Refetch the skeleton instead of reading the skeleton cache")
	skeletonViewCmd.Flags().BoolVar(&opts.JSONDebug, "json-debug", false, "Print skeleton JSON and do not open the GPU viewer")
	skeletonViewCmd.Flags().BoolVar(&opts.Wait, "wait", false, "Wait for uncached server-side skeleton generation instead of queueing and exiting")
	skeletonViewCmd.Flags().DurationVar(&opts.WaitTimeout, "wait-timeout", 10*time.Minute, "Maximum time to wait for uncached server-side skeleton generation")
	skeletonViewCmd.ValidArgsFunction = noFileCompletion
	skeletonViewCmd.RunE = func(cmd *cobra.Command, args []string) error {
		return runSkeletonView(cmd.Context(), cmd.OutOrStdout(), cmd.ErrOrStderr(), args[0], opts, defaultSkeletonViewDeps())
	}
	rootCmd.AddCommand(skeletonViewCmd)
}

func defaultSkeletonViewDeps() skeletonViewDeps {
	return skeletonViewDeps{
		token:      config.GetCAVEToken,
		lookPathUV: skeleton.LookPathUV,
		cacheRoot:  skeleton.DefaultCacheRoot,
		fetch:      skeleton.FetchWithBridge,
		viewerPath: findSkeletonViewer,
		launch:     launchSkeletonViewer,
		remote: func(token string) skeletonRemote {
			return skeleton.NewRemoteClient(config.CAVEServer, config.CAVESkeletonTable, token)
		},
		rootInfo: fetchSkeletonViewRootInfo,
		rootInfoOK: func() bool {
			return config.GetAPIToken() != "" && config.GetCAVEToken() != ""
		},
		sleep: sleepContext,
	}
}

func runSkeletonView(ctx context.Context, out, errOut io.Writer, rawRootID string, opts skeletonViewOptions, deps skeletonViewDeps) error {
	_, rootID, err := skeleton.ParseRootID(rawRootID)
	if err != nil {
		return err
	}
	projectionName := opts.Projection
	if projectionName == "" {
		projectionName = string(skeleton.Projection3D)
	}
	projection, err := skeleton.ParseProjection(projectionName)
	if err != nil {
		return err
	}
	maxNodes, err := skeleton.ParseMaxNodes(opts.MaxNodes)
	if err != nil {
		return err
	}
	if opts.Wait && opts.WaitTimeout <= 0 {
		return fmt.Errorf("--wait-timeout must be > 0")
	}
	token := ""
	if deps.token != nil {
		token = deps.token()
	}
	cacheRoot, err := deps.cacheRoot()
	if err != nil {
		return err
	}

	cache := skeleton.NewCache(cacheRoot)
	progress := newSkeletonProgress(errOut)
	var remote skeletonRemote
	if deps.remote != nil {
		remote = deps.remote(token)
	}
	sk, queued, err := skeletonForView(ctx, cache, rootID, token, deps.lookPathUV, opts.NoCache, opts.Wait, opts.WaitTimeout, deps.fetch, remote, deps.sleep, progress)
	if err != nil {
		return err
	}
	if queued != nil {
		return writeSkeletonQueued(out, errOut, rootID, queued, opts.JSONDebug)
	}
	sk = skeleton.LimitSkeleton(sk, maxNodes)

	if opts.JSONDebug {
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(sk)
	}
	rootInfo := deps.rootInfo
	if token == "" || (deps.rootInfoOK != nil && !deps.rootInfoOK()) {
		rootInfo = nil
	}
	infoPath, err := rootInfoForView(ctx, cache, rootID, !opts.NoCache, rootInfo, progress)
	if err != nil {
		return err
	}
	viewerPath, err := deps.viewerPath()
	if err != nil {
		return err
	}
	return progress.run(ctx, "opening GPU viewer", func() error {
		return deps.launch(ctx, viewerPath, cache.SkeletonPath(rootID), infoPath, string(projection), maxNodes)
	})
}

func rootInfoForView(
	ctx context.Context,
	cache skeleton.Cache,
	rootID string,
	useCache bool,
	fetchRootInfo func(string) ([]string, error),
	progress *skeletonProgress,
) (string, error) {
	info := skeleton.ViewerInfo{RootID: rootID}
	if useCache {
		var ok bool
		if err := progress.run(ctx, "checking root info cache", func() error {
			var err error
			info, ok, err = cache.ReadViewerInfo(rootID)
			return err
		}); err != nil {
			return "", err
		}
		if ok {
			progress.note("using cached root info")
			return cache.ViewerInfoPath(rootID), nil
		}
		info = skeleton.ViewerInfo{RootID: rootID}
	}
	if fetchRootInfo == nil {
		return "", nil
	}
	var fetchErr error
	if err := progress.run(ctx, "fetching root info", func() error {
		info.Lines, fetchErr = fetchRootInfo(rootID)
		return nil
	}); err != nil {
		return "", err
	}
	if fetchErr != nil {
		info.Error = fetchErr.Error()
	}
	if err := progress.run(ctx, "writing root info cache", func() error {
		return cache.WriteViewerInfo(info)
	}); err != nil {
		return "", err
	}
	return cache.ViewerInfoPath(rootID), nil
}

func fetchSkeletonViewRootInfo(rootID string) ([]string, error) {
	stClient, err := seatable.NewClient()
	if err != nil {
		return nil, err
	}
	caveClient, err := cave.NewClient()
	if err != nil {
		return nil, err
	}
	result, err := fetchRootInfo(seatableRootInfoSource{client: stClient}, caveClient, rootID, rootInfoOptions{
		HistoryLimit: 1,
		Filtered:     true,
	})
	if err != nil {
		return nil, err
	}
	return formatSkeletonViewRootInfo(result), nil
}

func formatSkeletonViewRootInfo(result *rootInfoResult) []string {
	if result == nil {
		return nil
	}
	classification := result.Classification
	lines := []string{"root_info"}
	if value := joinNonEmpty(" / ", classification.CellType, classification.CellSubtype); value != "" {
		lines = append(lines, "cell: "+value)
	}
	if value := joinNonEmpty(" / ", classification.SuperClass, classification.CellClass); value != "" {
		lines = append(lines, "class: "+value)
	}
	location := joinNonEmpty(" ", labeledValue("side", classification.Side), labeledValue("region", classification.Region), labeledValue("tract", classification.Tract), labeledValue("nerve", classification.Nerve))
	if location != "" {
		lines = append(lines, "location: "+location)
	}
	if strings.TrimSpace(classification.Proofread) != "" {
		lines = append(lines, "proofread: "+classification.Proofread)
	}
	if result.Position != nil {
		lines = append(lines, fmt.Sprintf("position: %.0f, %.0f, %.0f", result.Position.X, result.Position.Y, result.Position.Z))
	}
	caveStatus := joinNonEmpty(" ", result.CAVE.Status, labeledValue("current", result.CAVE.CurrentRootID), labeledValue("sv", result.CAVE.SupervoxelID))
	if caveStatus != "" {
		lines = append(lines, "cave: "+caveStatus)
	}
	if nearest := result.NearestColumn; nearest != nil {
		lines = append(lines, fmt.Sprintf("nearest: %s %s d=%.1f root=%s",
			displayValue(joinNonEmpty(" ", nearest.Region, nearest.Side)),
			displayValue(nearest.SideRelation),
			nearest.Distance,
			displayValue(nearest.RootID),
		))
	}
	history := result.History
	if history.Error != "" {
		lines = append(lines, "history: unavailable: "+history.Error)
		return lines
	}
	lines = append(lines, fmt.Sprintf("history: %d edits (%d merges, %d splits)", history.Total, history.Merges, history.Splits))
	if history.Latest != nil {
		latest := history.Latest
		lines = append(lines, fmt.Sprintf("latest: %s %s by %s", latest.TimestampUTC, latest.Type, displayValue(latest.UserName)))
	}
	return lines
}

func joinNonEmpty(separator string, values ...string) string {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			parts = append(parts, value)
		}
	}
	return strings.Join(parts, separator)
}

func labeledValue(label, value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	return label + "=" + value
}

func skeletonForView(
	ctx context.Context,
	cache skeleton.Cache,
	rootID string,
	token string,
	lookPathUV func() (string, error),
	noCache bool,
	wait bool,
	waitTimeout time.Duration,
	fetch func(context.Context, skeleton.BridgeOptions) (*skeleton.Skeleton, error),
	remote skeletonRemote,
	sleep func(context.Context, time.Duration) error,
	progress *skeletonProgress,
) (*skeleton.Skeleton, *skeletonQueued, error) {
	if sleep == nil {
		sleep = sleepContext
	}
	if !noCache {
		var (
			sk *skeleton.Skeleton
			ok bool
		)
		if err := progress.run(ctx, "checking skeleton cache", func() error {
			var err error
			sk, ok, err = cache.ReadSkeleton(rootID)
			return err
		}); err != nil {
			return nil, nil, err
		}
		if ok {
			progress.note(fmt.Sprintf("using cached skeleton (%d nodes, %d edges)", len(sk.Nodes), len(sk.Edges)))
			return sk, nil, nil
		}
	}
	if strings.TrimSpace(token) == "" {
		return nil, nil, missingCAVETokenError()
	}
	if remote == nil {
		return nil, nil, fmt.Errorf("skeleton remote dependency is not configured")
	}

	var exists bool
	if err := progress.run(ctx, "checking CAVE skeleton cache", func() error {
		var err error
		exists, err = remote.SkeletonExists(ctx, rootID)
		return err
	}); err != nil {
		return nil, nil, err
	}
	if !exists {
		queued := &skeletonQueued{}
		if err := progress.run(ctx, "queueing skeleton generation", func() error {
			var err error
			queued.EstimateSeconds, err = remote.QueueSkeleton(ctx, rootID)
			return err
		}); err != nil {
			return nil, nil, err
		}
		if !wait {
			return nil, queued, nil
		}
		if waitTimeout <= 0 {
			waitTimeout = 10 * time.Minute
		}
		progress.note("server cache miss; waiting for skeleton generation")
		waitCtx, cancel := context.WithTimeout(ctx, waitTimeout)
		defer cancel()

		backoff := 2 * time.Second
		for {
			if err := sleep(waitCtx, backoff); err != nil {
				if errors.Is(err, context.DeadlineExceeded) || errors.Is(waitCtx.Err(), context.DeadlineExceeded) {
					return nil, nil, fmt.Errorf("skeleton generation for root_id %s was queued but was not ready before --wait-timeout %s; run this command again later: %w", rootID, waitTimeout, context.DeadlineExceeded)
				}
				return nil, nil, err
			}
			if err := progress.run(waitCtx, "checking CAVE skeleton cache", func() error {
				var err error
				exists, err = remote.SkeletonExists(waitCtx, rootID)
				return err
			}); err != nil {
				if errors.Is(err, context.DeadlineExceeded) || errors.Is(waitCtx.Err(), context.DeadlineExceeded) {
					return nil, nil, fmt.Errorf("skeleton generation for root_id %s was queued but was not ready before --wait-timeout %s; run this command again later: %w", rootID, waitTimeout, context.DeadlineExceeded)
				}
				return nil, nil, err
			}
			if exists {
				break
			}
			backoff *= 2
			if backoff > 30*time.Second {
				backoff = 30 * time.Second
			}
		}
	}

	var sk *skeleton.Skeleton
	if err := progress.run(ctx, "fetching skeleton from CAVE", func() error {
		uvPath, err := lookPathUV()
		if err != nil {
			return err
		}
		var fetchErr error
		sk, fetchErr = fetch(ctx, skeleton.BridgeOptions{
			UVPath:     uvPath,
			RuntimeDir: cache.BridgeDir(),
			RootID:     rootID,
			Token:      token,
			Server:     config.CAVEServer,
			Datastack:  config.CAVESkeletonTable,
		})
		return fetchErr
	}); err != nil {
		return nil, nil, err
	}
	if sk.RootID == "" {
		sk.RootID = rootID
	}
	if err := skeleton.ValidateSkeleton(sk); err != nil {
		return nil, nil, err
	}
	if err := progress.run(ctx, "writing skeleton cache", func() error {
		return cache.WriteSkeleton(rootID, sk)
	}); err != nil {
		return nil, nil, err
	}
	return sk, nil, nil
}

func missingCAVETokenError() error {
	return fmt.Errorf("no CAVE token configured; run 'crantcli setup' or set CAVE_TOKEN")
}

func sleepContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

type skeletonQueued struct {
	EstimateSeconds float64
}

func writeSkeletonQueued(out, errOut io.Writer, rootID string, queued *skeletonQueued, asJSON bool) error {
	if asJSON {
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(map[string]any{
			"root_id":          rootID,
			"queued":           true,
			"estimate_seconds": queued.EstimateSeconds,
		})
	}
	if queued.EstimateSeconds > 0 {
		fmt.Fprintf(errOut, "Skeleton is not cached yet; queued generation for root_id %s. Estimated wait: %s.\n", rootID, formatProgressElapsed(time.Duration(queued.EstimateSeconds*float64(time.Second))))
	} else {
		fmt.Fprintf(errOut, "Skeleton is not cached yet; queued generation for root_id %s.\n", rootID)
	}
	fmt.Fprintf(errOut, "Run this again later to open it, or use --wait to block until CAVE finishes generating it.\n")
	return nil
}

func findSkeletonViewer() (string, error) {
	name := "crantcli-skeleton-viewer"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	if override := strings.TrimSpace(os.Getenv(skeletonViewerOverrideEnv)); override != "" {
		if !filepath.IsAbs(override) {
			return "", fmt.Errorf("%s must be an absolute path", skeletonViewerOverrideEnv)
		}
		if !isExecutableFile(override) {
			return "", fmt.Errorf("%s points to a non-executable file: %s", skeletonViewerOverrideEnv, override)
		}
		return override, nil
	}
	if exe, err := os.Executable(); err == nil {
		candidate := filepath.Join(filepath.Dir(exe), name)
		if isExecutableFile(candidate) {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("crantcli-skeleton-viewer helper not found next to crantcli; install the full release, set %s to an absolute helper path, or run `go install ./cmd/crantcli-skeleton-viewer`", skeletonViewerOverrideEnv)
}

func isExecutableFile(path string) bool {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	if runtime.GOOS == "windows" {
		return true
	}
	return info.Mode()&0o111 != 0
}

func launchSkeletonViewer(ctx context.Context, viewerPath, skeletonPath, infoPath, projection string, maxNodes int) error {
	args := []string{
		"--projection", projection,
		"--max-nodes", strconv.Itoa(maxNodes),
	}
	if infoPath != "" {
		args = append(args, "--info", infoPath)
	}
	args = append(args, skeletonPath)
	cmd := exec.CommandContext(ctx, viewerPath, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = skeletonViewerEnv(os.Environ())
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("running skeleton viewer: %w", err)
	}
	return nil
}

var skeletonViewerAllowedEnv = map[string]bool{
	"APPDATA":                     true,
	"COLORTERM":                   true,
	"COMSPEC":                     true,
	"DBUS_SESSION_BUS_ADDRESS":    true,
	"DISPLAY":                     true,
	"DRI_PRIME":                   true,
	"HOME":                        true,
	"LANG":                        true,
	"LANGUAGE":                    true,
	"LIBGL_ALWAYS_SOFTWARE":       true,
	"LOCALAPPDATA":                true,
	"LOGNAME":                     true,
	"MESA_LOADER_DRIVER_OVERRIDE": true,
	"PATH":                        true,
	"PATHEXT":                     true,
	"SHELL":                       true,
	"SYSTEMROOT":                  true,
	"TEMP":                        true,
	"TERM":                        true,
	"TMP":                         true,
	"TMPDIR":                      true,
	"USER":                        true,
	"USERNAME":                    true,
	"USERPROFILE":                 true,
	"WAYLAND_DISPLAY":             true,
	"WINDIR":                      true,
	"XAUTHORITY":                  true,
	"XDG_RUNTIME_DIR":             true,
	"XDG_SESSION_TYPE":            true,
	"XPC_FLAGS":                   true,
	"XPC_SERVICE_NAME":            true,
	"__CF_USER_TEXT_ENCODING":     true,
}

func skeletonViewerEnv(base []string) []string {
	return filterAllowedEnv(base, skeletonViewerAllowedEnv, []string{"LC_"})
}

func filterAllowedEnv(base []string, allowed map[string]bool, allowedPrefixes []string) []string {
	env := make([]string, 0, len(base))
	for _, entry := range base {
		name, _, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		upperName := strings.ToUpper(name)
		if allowed[upperName] {
			env = append(env, entry)
			continue
		}
		for _, prefix := range allowedPrefixes {
			if strings.HasPrefix(upperName, prefix) {
				env = append(env, entry)
				break
			}
		}
	}
	return env
}

type skeletonProgress struct {
	w           io.Writer
	interactive bool
}

func newSkeletonProgress(w io.Writer) *skeletonProgress {
	if w == nil {
		w = io.Discard
	}
	p := &skeletonProgress{w: w}
	if f, ok := w.(*os.File); ok {
		p.interactive = term.IsTerminal(int(f.Fd()))
	}
	return p
}

func (p *skeletonProgress) note(message string) {
	if !p.interactive {
		return
	}
	fmt.Fprintf(p.w, "[ok] %s\n", message)
}

func (p *skeletonProgress) run(ctx context.Context, message string, fn func() error) error {
	if !p.interactive {
		return fn()
	}

	done := make(chan error, 1)
	start := time.Now()
	go func() {
		done <- fn()
	}()

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	frame := 0
	lastWidth := 0
	render := func(status string) {
		line := progressLine(status, message, time.Since(start), frame)
		if len(line) < lastWidth {
			line += strings.Repeat(" ", lastWidth-len(line))
		}
		lastWidth = len(line)
		fmt.Fprintf(p.w, "\r%s", line)
	}

	render("..")
	for {
		select {
		case err := <-done:
			status := "ok"
			if err != nil {
				status = "!!"
			}
			render(status)
			fmt.Fprintln(p.w)
			return err
		case <-ticker.C:
			frame++
			render("..")
		case <-ctx.Done():
			render("!!")
			fmt.Fprintln(p.w)
			return ctx.Err()
		}
	}
}

func progressLine(status, message string, elapsed time.Duration, frame int) string {
	const width = 18
	pos := frame % width
	bar := make([]byte, width)
	for i := range bar {
		bar[i] = '-'
	}
	bar[pos] = '>'
	return fmt.Sprintf("[%s] [%s] %s %s", status, string(bar), message, formatProgressElapsed(elapsed))
}

func formatProgressElapsed(elapsed time.Duration) string {
	elapsed = elapsed.Round(time.Second)
	minutes := int(elapsed / time.Minute)
	seconds := int((elapsed % time.Minute) / time.Second)
	if minutes > 0 {
		return fmt.Sprintf("%dm%02ds", minutes, seconds)
	}
	return fmt.Sprintf("%ds", seconds)
}
