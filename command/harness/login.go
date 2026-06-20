package harness

import (
	"context"
	"fmt"

	"github.com/privateerproj/privateer-sdk/internal/auth"
	"github.com/privateerproj/privateer-sdk/internal/oci"
	"github.com/spf13/cobra"
)

// loginCmd returns `pvtr login` — an interactive OIDC device-grant against
// the hub's advertised auth server, storing a token `pvtr publish` reads. The
// hub + its OIDC coordinates come from discovery against the configured hub
// (the hub-url config key / PVTR_HUB_URL), so the user supplies no URLs. CI does
// not use this — it sets PVTR_TOKEN instead.
func loginCmd(writerFn func() Writer) *cobra.Command {
	loginCmd := &cobra.Command{
		Use:   "login",
		Short: "Sign in to grc.store (OIDC device grant) for `pvtr publish`.",
		Long: "Run an OAuth 2.0 device-authorization flow against the auth server the hub " +
			"advertises (hub-url config / PVTR_HUB_URL → discovery → oidc_issuer / oidc_cli_client_id) " +
			"and store the token so `pvtr publish` can authenticate. In CI, set PVTR_TOKEN instead of logging in.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runLogin(cmd.Context(), writerFn())
		},
	}
	return loginCmd
}

func runLogin(ctx context.Context, writer Writer) error {
	defer func() { _ = writer.Flush() }()
	if ctx == nil {
		ctx = context.Background()
	}
	disco, err := oci.NewClient().Discover(ctx)
	if err != nil {
		return fmt.Errorf("hub discovery (hub %s): %w", oci.HubURL(), err)
	}
	if disco.OIDCIssuer == "" {
		return fmt.Errorf("the hub at %s does not advertise an OIDC issuer; `pvtr login` is not supported there", oci.HubURL())
	}
	issuer, err := auth.Login(ctx, disco.OIDCIssuer, disco.OIDCClientID, writer)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(writer, "Logged in to %s.\n", issuer)
	return nil
}

// logoutCmd returns `pvtr logout` — forgets the stored credentials for the
// hub's issuer.
func logoutCmd(writerFn func() Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Forget stored grc.store credentials.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			writer := writerFn()
			defer func() { _ = writer.Flush() }()
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			disco, err := oci.NewClient().Discover(ctx)
			if err != nil {
				return fmt.Errorf("hub discovery: %w", err)
			}
			if err := auth.Logout(disco.OIDCIssuer); err != nil {
				return err
			}
			_, _ = fmt.Fprintln(writer, "Logged out.")
			return nil
		},
	}
}
