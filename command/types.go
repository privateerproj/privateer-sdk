package command

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"

	hclog "github.com/hashicorp/go-hclog"
	hcplugin "github.com/hashicorp/go-plugin"
	"github.com/spf13/viper"

	"github.com/privateerproj/privateer-sdk/config"
	"github.com/privateerproj/privateer-sdk/internal/manifest"
)

// PluginError retains an error object and the name of the pack that generated it.
type PluginError struct {
	Plugin string
	Err    error
}

// PluginErrors holds a list of errors and an Error() method
// so it adheres to the standard Error interface.
type PluginErrors struct {
	Errors []PluginError
}

func (e *PluginErrors) Error() string {
	return fmt.Sprintf("Service Pack Errors: %v", e.Errors)
}

// PluginPkg represents a plugin package with its metadata and execution state.
type PluginPkg struct {
	Name          string
	Version       string // requested version; "" means "latest installed"
	Path          string
	ServiceTarget string
	Command       *exec.Cmd
	Result        string

	Installable bool
	Installed   bool
	Requested   bool
	Successful  bool
	Error       error

	entry *manifest.Plugin // manifest entry the binary was resolved from
}

// getBinary resolves the on-disk path of the plugin binary from the manifest.
// When Version is set it requires that exact version; otherwise it selects the
// latest installed version. This replaces filesystem-glob discovery so that
// multiple installed versions (each at its own coordinate/version/entrypoint
// path) resolve unambiguously by name+version.
//
// It applies the require-signed-plugins policy, which needs no I/O, so a
// refused plugin fails the run before anything starts. The binary hash runs
// later, in runOne, just before the plugin is exec'd.
func (p *PluginPkg) getBinary() (binaryPath string, err error) {
	binariesPath := config.GetBinariesPath()
	m, err := manifest.Load(binariesPath)
	if err != nil {
		return "", fmt.Errorf("loading plugin manifest: %w", err)
	}

	var entry *manifest.Plugin
	if p.Version != "" {
		if entry = m.FindVersion(p.Name, p.Version); entry == nil {
			return "", fmt.Errorf("plugin %s@%s is not installed in %s", p.Name, p.Version, binariesPath)
		}
	} else if entry = m.Latest(p.Name); entry == nil {
		return "", fmt.Errorf("plugin %s is not installed in %s", p.Name, binariesPath)
	}
	if err := checkSignedPolicy(entry, config.RequireSignedPlugins()); err != nil {
		return "", err
	}
	p.entry = entry
	return filepath.Join(binariesPath, entry.BinaryPath), nil
}

// ErrPluginVerification marks a plugin that is installed but was refused by the
// execution-time check (binary digest mismatch, or unsigned under
// require-signed-plugins). It is reported as a refusal, not as "not
// installed", so the user sees why the plugin was blocked.
var ErrPluginVerification = errors.New("plugin failed verification")

// What this check can and cannot do: the recorded digest and signer fields are
// read from plugins.json in the same directory as the binary, so it catches
// accidental or out-of-band changes to the binary. It does not stop anyone
// who can write the binaries directory, since they can edit the manifest too.
// Keep that directory writable only by the user running pvtr. "Signed" below
// means "plugins.json says the install was signature-verified", nothing more.

// checkSignedPolicy applies require-signed-plugins to a manifest entry. It
// refuses entries not signature-verified at install (local installs) and
// entries that predate binary-digest recording. It does no I/O.
func checkSignedPolicy(entry *manifest.Plugin, requireSigned bool) error {
	if !requireSigned {
		return nil
	}
	ref := entry.Name + "@" + entry.Version
	if !entry.Signed() {
		return fmt.Errorf("%w: %s is not signed (installed from a local binary or without verification) and require-signed-plugins is enabled", ErrPluginVerification, ref)
	}
	if entry.BinaryDigest == "" {
		return fmt.Errorf("%w: %s has no recorded binary digest (installed by an older pvtr); %s to run with require-signed-plugins enabled", ErrPluginVerification, ref, reinstallHint(entry))
	}
	return nil
}

// verifyBinaryDigest re-hashes the on-disk binary against the digest recorded
// at install, so a binary changed after install is refused rather than run.
// An entry with no recorded digest (older pvtr) has nothing to check against.
func verifyBinaryDigest(entry *manifest.Plugin, binaryPath string) error {
	if entry.BinaryDigest == "" {
		return nil
	}
	ref := entry.Name + "@" + entry.Version
	got, err := manifest.DigestFile(binaryPath)
	if err != nil {
		return fmt.Errorf("%w: hashing %s binary: %v", ErrPluginVerification, ref, err)
	}
	if got != entry.BinaryDigest {
		return fmt.Errorf("%w: %s binary at %s does not match the digest recorded at install (got %s, want %s); it may have been tampered with. Refusing to run: %s (autoinstall will not replace an installed plugin)", ErrPluginVerification, ref, binaryPath, got, entry.BinaryDigest, reinstallHint(entry))
	}
	return nil
}

// verifyInstalledBinary runs the full check: policy, then digest.
func verifyInstalledBinary(entry *manifest.Plugin, binaryPath string, requireSigned bool) error {
	if err := checkSignedPolicy(entry, requireSigned); err != nil {
		return err
	}
	return verifyBinaryDigest(entry, binaryPath)
}

// reinstallHint names the command that repairs an entry.
func reinstallHint(entry *manifest.Plugin) string {
	if entry.Coordinate == "" {
		return "reinstall it from its original local binary"
	}
	return fmt.Sprintf("reinstall it with `pvtr install %s@%s`", entry.Coordinate, entry.Version)
}

// verifyBeforeExec is the last check before the plugin starts. It records a
// failure on p.Error.
func (p *PluginPkg) verifyBeforeExec() error {
	if p.entry == nil {
		return nil
	}
	if err := verifyBinaryDigest(p.entry, p.Path); err != nil {
		p.Error = err
		return err
	}
	return nil
}

func (p *PluginPkg) queueCmd() {
	cmd := exec.Command(p.Path)
	cmd.Args = append(cmd.Args,
		fmt.Sprintf("--config=%s", viper.GetString("config")),
		fmt.Sprintf("--loglevel=%s", viper.GetString("loglevel")),
		fmt.Sprintf("--service=%s", p.ServiceTarget),
	)
	// Harness-resolved values go to the plugin through the environment (viper
	// maps each key to PVTR_<KEY>), not as flags: a plugin built on an older SDK
	// fails on a flag it does not register but simply ignores an env var.
	// binaries-path is where the plugin finds the catalogs `pvtr install`
	// cached for it; the rest are run flags the plugin reads when writing
	// results and would otherwise fall back to its own defaults.
	cmd.Env = append(os.Environ(),
		"PVTR_BINARIES_PATH="+config.GetBinariesPath(),
		"PVTR_WRITE_DIRECTORY="+viper.GetString("write-directory"),
		"PVTR_OUTPUT="+viper.GetString("output"),
		"PVTR_WRITE="+strconv.FormatBool(viper.GetBool("write")),
		"PVTR_INCLUDE_PAYLOAD="+strconv.FormatBool(viper.GetBool("include-payload")),
	)
	p.Command = cmd
}

// closeClient logs the plugin result and kills the process.
func (p *PluginPkg) closeClient(serviceName string, client *hcplugin.Client, logger hclog.Logger) {
	if p.Successful {
		logger.Info(fmt.Sprintf("Plugin for %s completed successfully", serviceName))
	} else if p.Error != nil {
		logger.Error(fmt.Sprintf("Error from %s: %s", serviceName, p.Error))
	} else {
		logger.Warn(fmt.Sprintf("Unexpected exit from %s with no error or success", serviceName))
	}
	client.Kill()
}

// NewPluginPkg creates a new PluginPkg for the given plugin name, requested
// version (empty for "latest installed"), and service. It resolves the binary
// from the manifest: on success the package is marked Installed with its Path
// set; on failure Error is recorded and Installed stays false.
func NewPluginPkg(pluginName, version, serviceName string) *PluginPkg {
	plugin := &PluginPkg{
		Name:          pluginName,
		Version:       version,
		ServiceTarget: serviceName,
	}
	path, err := plugin.getBinary()
	if err != nil {
		plugin.Error = err
	} else {
		plugin.Path = path
		plugin.Installed = true
		plugin.queueCmd()
	}
	return plugin
}
