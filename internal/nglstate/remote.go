package nglstate

import (
	"errors"
	"fmt"
)

// RemoteState is a state stored on a server instead of embedded in a URL. A
// Neuroglancer Share link names one in its fragment, for example
// #!middleauth+https://global.daf-apis.com/nglstate/api/v1/<id>.
type RemoteState struct {
	// Location is the fragment after "#!", exactly as the link spells it, or
	// percent-decoded when only the decoded fragment names a location.
	Location string
}

// RemoteStateResolver returns the state JSON stored at a remote location. For
// a location it cannot fetch, such as one on a scheme it does not handle, it
// returns an error wrapping errors.ErrUnsupported.
type RemoteStateResolver func(RemoteState) ([]byte, error)

// resolveRemoteState stays nil unless a command installs a resolver, so this
// package never reaches the network or reads credentials itself.
var resolveRemoteState RemoteStateResolver

// SetRemoteStateResolver installs the function that fetches the states
// Neuroglancer Share links point to. Without one, or for a location it does
// not support, loading a Share link fails with an error that names the link
// and how to get a URL that embeds the state.
func SetRemoteStateResolver(resolve RemoteStateResolver) {
	resolveRemoteState = resolve
}

// remoteFetchError reports a Share link whose state the resolver could not
// fetch, or fetched but could not parse.
type remoteFetchError struct {
	location string
	err      error
}

func (e *remoteFetchError) Error() string {
	return fmt.Sprintf("the state stored at %q could not be loaded: %v", e.location, e.err)
}

func (e *remoteFetchError) Unwrap() error {
	return e.err
}

// decodeStateURL reads the state in a Neuroglancer URL the way the viewer
// does: from the fragment, or for a Share link, from where the fragment says
// it is stored. remote is set only when the state was fetched. Without a
// resolver that can fetch the location, the error is DecodeURL's, which says
// how to get a URL that embeds the state.
func decodeStateURL(rawURL string) (state map[string]interface{}, remote *RemoteState, err error) {
	state, err = DecodeURL(rawURL)
	var stored *remoteStateError
	if !errors.As(err, &stored) || resolveRemoteState == nil {
		return state, nil, err
	}

	location := RemoteState{Location: stored.location}
	data, err := resolveRemoteState(location)
	if errors.Is(err, errors.ErrUnsupported) {
		return nil, nil, stored
	}
	if err != nil {
		return nil, nil, &remoteFetchError{location: stored.location, err: err}
	}
	state, err = parseStateJSON(string(data))
	if err != nil {
		return nil, nil, &remoteFetchError{location: stored.location, err: fmt.Errorf("it is not a Neuroglancer state: %w", err)}
	}
	return state, &location, nil
}
