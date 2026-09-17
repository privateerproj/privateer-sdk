// SPDX-License-Identifier: Apache-2.0

package oci

import (
	"context"
	"fmt"

	ckbundle "github.com/gemaraproj/grc-store-clientkit/bundle"
	"github.com/gemaraproj/grc-store-clientkit/keyless"
	"github.com/revanite-io/grc-store-protocol/mediatype"
)

// SignAndAttach signs the assembled index keyless and attaches the resulting
// Sigstore bundle as an OCI 1.1 referrer of the index — the step `cosign sign`
// would otherwise perform.
//
// The signed payload is a DSSE-wrapped in-toto Statement v1 whose single
// subject digest is the index digest.
//
// idToken is the public-good Fulcio identity from keyless.Identity. It is NOT
// the hub bearer: Fulcio trusts public OIDC issuers, not the grc.store auth
// server, so the two are resolved separately and must never be conflated.
func SignAndAttach(ctx context.Context, idx *AssembledIndex, push PushOptions, idToken string) error {
	sig, err := keyless.Signer{IDToken: idToken}.Sign(ctx, idx.IndexDigest(), "", nil)
	if err != nil {
		return err
	}
	repo, err := newPluginRepository(push, idx.Coordinate)
	if err != nil {
		return err
	}
	if err := ckbundle.AttachReferrer(ctx, repo, idx.Index.descriptor(), mediatype.SigstoreBundle, sig); err != nil {
		return fmt.Errorf("attaching signature to %s: %w", idx.Coordinate, err)
	}
	return nil
}
