package cmd

import (
	"fmt"

	"github.com/CarlosHPlata/shrine/internal/app"
	"github.com/CarlosHPlata/shrine/internal/handler"
	"github.com/CarlosHPlata/shrine/internal/manifest"
	"github.com/spf13/cobra"
)

var (
	bumpVersion string
	bumpTeam    string
	bumpPath    string
	bumpDryRun  bool
)

var bumpCmd = &cobra.Command{
	Use:   "bump",
	Short: "Move a pinned artifact to another version",
	Long: `Move a Pinned application or resource to a chosen version, or to the newest
one, by recording a new pin. Nothing is started, stopped, or recreated: the
next "shrine deploy" applies the pin. Rolling back is a bump to an earlier
version.`,
}

var bumpApplicationCmd = &cobra.Command{
	Use:     "application [name]",
	Aliases: []string{"app"},
	Short:   "Pin an application to a chosen or the newest version",
	Long:    bumpLong("an", "application"),
	Args:    cobra.ExactArgs(1),
	RunE:    runBump(manifest.ApplicationKind),
}

var bumpResourceCmd = &cobra.Command{
	Use:     "resource [name]",
	Aliases: []string{"res"},
	Short:   "Pin a resource to a chosen or the newest version",
	Long:    bumpLong("a", "resource"),
	Args:    cobra.ExactArgs(1),
	RunE:    runBump(manifest.ResourceKind),
}

// bumpLong renders the shared help text with the kind word substituted.
func bumpLong(article, kind string) string {
	return fmt.Sprintf(`Resolve the chosen version of a Pinned %[2]s in the registry and record
it as the %[2]s's new pin. Nothing is started, stopped, or recreated:
the next "shrine deploy" applies the pin.

-v names the version to pin: a readable version (a tag such as 17 or v1.4.0)
or an exact version (sha256:…). The repository always comes from the manifest,
so a bump can never point %[1]s %[2]s at a different image. Without -v the
newest version of the manifest's image is resolved and pinned. Rolling back is
a bump to an earlier version.

Only %[2]ss whose effective image pull policy is Pinned can be bumped;
under Always or IfNotPresent the version is manifest-owned and the bump
refuses. The %[2]s need not be deployed: a pin recorded before the first
deploy is the version that deploy runs. The manifest is looked up in the
specs directory (or --path); --team verifies the manifest's owner.

The output states the previous and the new version. --dry-run prints what
would be resolved and pinned and writes nothing.`, article, kind)
}

func runBump(kind string) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		dir, err := cfg.ResolveSpecsDir(bumpPath)
		if err != nil {
			return err
		}
		opts := handler.BumpOptions{Kind: kind, Name: args[0], Team: bumpTeam, Version: bumpVersion}
		if bumpDryRun {
			return handler.BumpDryRun(cmd.OutOrStdout(), cmd.ErrOrStderr(), dir, store, cfg, opts)
		}
		bundle, cleanup, err := app.BuildBumpBundle(cfg, store, paths, dir, cmd.OutOrStdout(), cmd.ErrOrStderr())
		if err != nil {
			return err
		}
		defer cleanup()
		return handler.Bump(bundle, opts)
	}
}

func init() {
	bumpCmd.PersistentFlags().StringVarP(&bumpVersion, "version", "v", "", "Readable version (tag) or exact version (sha256:…) to pin; newest when omitted")
	bumpCmd.PersistentFlags().StringVarP(&bumpTeam, "team", "t", "", "Team owning the artifact (verified when given)")
	bumpCmd.PersistentFlags().StringVarP(&bumpPath, "path", "p", "", "Directory containing manifest files (overrides specsDir in config.yml)")
	bumpCmd.PersistentFlags().BoolVar(&bumpDryRun, "dry-run", false, "Print what would be resolved and pinned without changing state")
	bumpCmd.AddCommand(bumpApplicationCmd)
	bumpCmd.AddCommand(bumpResourceCmd)
	rootCmd.AddCommand(bumpCmd)
}
