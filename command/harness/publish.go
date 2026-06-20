package harness

import (
	"github.com/privateerproj/privateer-sdk/internal/publish"
	"github.com/spf13/cobra"
)

// publishCmd returns the `pvtr publish` command — the complete one-command
// producer: assemble a multi-platform OCI index from a GoReleaser dist dir,
// authenticated-push it to the hub's registry, keyless-sign it against
// public-good Sigstore and attach the signature as the index's OCI referrer,
// then /sync so the hub ingests + verifies it.
//
// The plugin coordinate and the control-catalog linkage it evaluates are NOT
// flags — they are read from the built plugin itself (the publish-manifest
// subcommand), so the data lives in the plugin's source (its embedded catalogs
// + the orchestrator.Publisher field) and can't be forged at publish time by
// someone who doesn't own the plugin.
//
// Two identities are involved (inherent to keyless signing): the REGISTRY/HUB
// bearer (push + sync) comes from `pvtr login` (Keycloak) or CI's PVTR_TOKEN;
// the SIGNING identity (the Fulcio cert) comes from a public-good-trusted OIDC
// issuer — a GitHub Actions OIDC token (audience "sigstore") in CI, or a second
// interactive browser sign-in for a human. They are NOT interchangeable.
//
// The publish logic lives in internal/publish; this is the CLI seam that owns
// the writer and maps flags to publish.Params.
func publishCmd(writerFn func() Writer) *cobra.Command {
	var (
		distDir  string
		registry string
		noSync   bool
	)

	publishCmd := &cobra.Command{
		Use:   "publish",
		Short: "Assemble, push, and sync a plugin's OCI index to grc.store.",
		Long: "Assemble a multi-platform OCI plugin index from a GoReleaser dist directory, " +
			"push it to the grc.store registry (discovered from the hub-url config / PVTR_HUB_URL, default " +
			"https://hub.grc.store), and POST /sync so the hub ingests and verifies it.\n\n" +
			"The plugin's coordinate and evaluated catalogs are read from the built binary " +
			"itself (its publish-manifest), not from flags — set orchestrator.Publisher in the " +
			"plugin (coordinate = <publisher>/<plugin-name>).\n\n" +
			"Authenticate first with `pvtr login` (interactive device grant); in CI set " +
			"PVTR_TOKEN to a GitHub-Actions OIDC token (trusted publishing). Use --registry " +
			"to push to a different host (e.g. a local zot or GHCR) for testing — that path " +
			"is anonymous and skips sync.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			w := writerFn()
			defer func() { _ = w.Flush() }()
			return publish.Publish(cmd.Context(), w, publish.Params{
				DistDir:  distDir,
				Registry: registry,
				NoSync:   noSync,
			})
		},
	}
	publishCmd.Flags().StringVar(&distDir, "dist", "dist", "GoReleaser dist directory (contains artifacts.json + metadata.json)")
	publishCmd.Flags().StringVar(&registry, "registry", "", "registry override WITH scheme for testing, e.g. http://localhost:5000 or https://ghcr.io/<owner> (anonymous, skips signing + sync)")
	publishCmd.Flags().BoolVar(&noSync, "no-sync", false, "push-only smoke: skip signing and /sync (no signing identity needed)")
	return publishCmd
}
