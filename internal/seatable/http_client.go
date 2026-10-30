// Copyright 2026 Yigit Yargili
// SPDX-License-Identifier: Apache-2.0

package seatable

import "crantcli/internal/httpx"

// httpClient is the shared HTTP client used for SeaTable requests. It reuses
// the common httpx client so the timeout is configured in one place.
var httpClient = httpx.DefaultClient
