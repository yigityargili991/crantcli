package cmd

import (
	"fmt"
	"os"

	"github.com/sigstore/sigstore-go/pkg/bundle"
	"github.com/sigstore/sigstore-go/pkg/root"
	"github.com/sigstore/sigstore-go/pkg/tuf"
	"github.com/sigstore/sigstore-go/pkg/verify"
	"github.com/spf13/cobra"
)

var fetchUpdateTrustedRoot = func() (*root.TrustedRoot, error) {
	return root.FetchTrustedRootWithOptions(tuf.DefaultOptions().WithDisableLocalCache())
}

var verifyReleaseCmd = &cobra.Command{
	Use:    "__verify-release ARTIFACT BUNDLE",
	Hidden: true,
	Args:   cobra.ExactArgs(2),
	RunE: func(_ *cobra.Command, args []string) error {
		return verifyReleaseArtifact(args[0], args[1])
	},
}

func init() {
	rootCmd.AddCommand(verifyReleaseCmd)
}

func verifyReleaseArtifact(artifactPath, bundlePath string) error {
	signedBundle, err := bundle.LoadJSONFromPath(bundlePath)
	if err != nil {
		return fmt.Errorf("load Sigstore bundle: %w", err)
	}
	trustedRoot, err := fetchUpdateTrustedRoot()
	if err != nil {
		return fmt.Errorf("load Sigstore trusted root: %w", err)
	}
	verifier, err := verify.NewVerifier(
		trustedRoot,
		verify.WithSignedCertificateTimestamps(1),
		verify.WithTransparencyLog(1),
		verify.WithObserverTimestamps(1),
	)
	if err != nil {
		return fmt.Errorf("create Sigstore verifier: %w", err)
	}
	identity, err := verify.NewShortCertificateIdentity(
		updateCertificateIssuer, "", "", updateCertificateIdentity,
	)
	if err != nil {
		return fmt.Errorf("configure release identity: %w", err)
	}
	artifact, err := os.Open(artifactPath)
	if err != nil {
		return fmt.Errorf("open release artifact: %w", err)
	}
	defer artifact.Close()

	if _, err := verifier.Verify(
		signedBundle,
		verify.NewPolicy(verify.WithArtifact(artifact), verify.WithCertificateIdentity(identity)),
	); err != nil {
		return fmt.Errorf("verify Sigstore bundle: %w", err)
	}
	return nil
}
