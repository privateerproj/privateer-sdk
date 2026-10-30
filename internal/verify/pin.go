package verify

import "fmt"

// PinnedIdentityFor determines the effective PinnedIdentity to enforce and
// any warning to surface, applying the three-tier precedence:
//
//  1. Local pin (non-empty localPin) — always wins.
//  2. Hub-declared signer identity (hubIdentity) — used on first install.
//  3. Open TOFU (empty) — only when both are absent.
//
// When a local pin and hubIdentity are both non-empty and differ, a warning
// message is returned so the caller can inform the user; the local pin is
// still enforced. The actionHint is appended to the warning (e.g.
// "uninstall first to accept the new identity").
func PinnedIdentityFor(localPin, hubIdentity, actionHint string) (pin string, warn string) {
	if localPin != "" {
		if hubIdentity != "" && hubIdentity != localPin {
			warn = fmt.Sprintf(
				"local signer pin %q differs from hub-declared identity %q — "+
					"enforcing local pin; %s",
				localPin, hubIdentity, actionHint,
			)
		}
		return localPin, warn
	}
	// No local pin: seed from hub identity (may be empty → open TOFU).
	return hubIdentity, ""
}

