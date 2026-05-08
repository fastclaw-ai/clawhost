package cmd

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/clawhost/clawhost/service/k8s"
	"github.com/spf13/cobra"
)

var pvcCmd = &cobra.Command{
	Use:   "pvc",
	Short: "Read/modify/delete files in a bot's PVC",
	Long: `Operate on a bot's PVC contents (ls/cat/cp/rm/edit).

Paths are relative to the bot's data root (which is /home/node/.openclaw
inside the bot pod). When the bot pod is running, commands run there;
otherwise a short-lived alpine pod is spawned with the same PVC + subPath.`,
}

var pvcLsCmd = &cobra.Command{
	Use:   "ls <bot-id> [path]",
	Short: "List a directory in the bot's PVC",
	Args:  cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		botID := args[0]
		rel := ""
		if len(args) == 2 {
			rel = args[1]
		}
		ap, err := openAccess(botID)
		if err != nil {
			return err
		}
		defer ap.Close()
		abs, err := ap.AbsPath(rel)
		if err != nil {
			return err
		}
		return k8s.ExecStream(cmd.Context(), ap.Namespace, ap.Name, ap.Container,
			[]string{"ls", "-la", abs},
			k8s.ExecStreamOptions{Stdout: os.Stdout, Stderr: os.Stderr})
	},
}

var pvcCatCmd = &cobra.Command{
	Use:   "cat <bot-id> <path>",
	Short: "Print a file from the bot's PVC",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		botID, rel := args[0], args[1]
		ap, err := openAccess(botID)
		if err != nil {
			return err
		}
		defer ap.Close()
		abs, err := ap.AbsPath(rel)
		if err != nil {
			return err
		}
		return k8s.ExecStream(cmd.Context(), ap.Namespace, ap.Name, ap.Container,
			[]string{"cat", abs},
			k8s.ExecStreamOptions{Stdout: os.Stdout, Stderr: os.Stderr})
	},
}

var pvcRmCmd = &cobra.Command{
	Use:   "rm [-r] [-f] <bot-id> <path>",
	Short: "Remove a file or directory in the bot's PVC",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		recursive, _ := cmd.Flags().GetBool("recursive")
		force, _ := cmd.Flags().GetBool("force")
		botID, rel := args[0], args[1]
		if rel == "" || rel == "." {
			return fmt.Errorf("refusing to remove the bot data root")
		}
		ap, err := openAccess(botID)
		if err != nil {
			return err
		}
		defer ap.Close()
		abs, err := ap.AbsPath(rel)
		if err != nil {
			return err
		}
		rmArgs := []string{"rm"}
		if recursive {
			rmArgs = append(rmArgs, "-r")
		}
		if force {
			rmArgs = append(rmArgs, "-f")
		}
		rmArgs = append(rmArgs, abs)
		return k8s.ExecStream(cmd.Context(), ap.Namespace, ap.Name, ap.Container, rmArgs,
			k8s.ExecStreamOptions{Stdout: os.Stdout, Stderr: os.Stderr})
	},
}

var pvcCpCmd = &cobra.Command{
	Use:   "cp <src> <dst>",
	Short: "Copy a file in or out of a bot's PVC",
	Long: `Copy a file in or out of a bot's PVC. Use bot-id:path for the remote side.

Examples:
  clawhost pvc cp abc12345:openclaw.json ./openclaw.json
  clawhost pvc cp ./skill.md abc12345:skills/my-skill/skill.md`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runPvcCp(cmd.Context(), args[0], args[1])
	},
}

var pvcEditCmd = &cobra.Command{
	Use:   "edit <bot-id> <path>",
	Short: "Edit a file in a bot's PVC using $EDITOR",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runPvcEdit(cmd.Context(), args[0], args[1])
	},
}

func init() {
	pvcRmCmd.Flags().BoolP("recursive", "r", false, "remove directories recursively")
	pvcRmCmd.Flags().BoolP("force", "f", false, "ignore nonexistent files")

	pvcCmd.AddCommand(pvcLsCmd, pvcCatCmd, pvcRmCmd, pvcCpCmd, pvcEditCmd)
	rootCmd.AddCommand(pvcCmd)
}

func openAccess(botID string) (*k8s.AccessPod, error) {
	if err := initConfigLight(); err != nil {
		return nil, err
	}
	if err := k8s.InitClient(); err != nil {
		return nil, err
	}
	return k8s.OpenAccess(context.Background(), botID)
}

// parseRemote splits "bot-id:path" into (bot-id, path). Returns ("", s) for
// pure local paths. Heuristic: anything before ':' that contains '/' or '\\'
// is treated as a local path (e.g. "./foo:bar" or Windows paths).
func parseRemote(s string) (botID, p string) {
	i := strings.Index(s, ":")
	if i <= 0 {
		return "", s
	}
	head := s[:i]
	if strings.ContainsAny(head, `/\`) {
		return "", s
	}
	return head, s[i+1:]
}

func runPvcCp(ctx context.Context, src, dst string) error {
	srcBot, srcPath := parseRemote(src)
	dstBot, dstPath := parseRemote(dst)
	if (srcBot == "") == (dstBot == "") {
		return fmt.Errorf("exactly one of <src>/<dst> must be a remote bot path (bot-id:path)")
	}
	botID := srcBot + dstBot
	ap, err := openAccess(botID)
	if err != nil {
		return err
	}
	defer ap.Close()

	if srcBot != "" {
		abs, err := ap.AbsPath(srcPath)
		if err != nil {
			return err
		}
		return k8s.CopyFromPod(ctx, ap, abs, dstPath)
	}
	abs, err := ap.AbsPath(dstPath)
	if err != nil {
		return err
	}
	return k8s.CopyToPod(ctx, ap, srcPath, abs)
}

func runPvcEdit(ctx context.Context, botID, rel string) error {
	ap, err := openAccess(botID)
	if err != nil {
		return err
	}
	defer ap.Close()

	abs, err := ap.AbsPath(rel)
	if err != nil {
		return err
	}

	tmp, err := os.CreateTemp("", "clawhost-edit-*"+filepath.Ext(rel))
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	tmp.Close()
	defer os.Remove(tmpPath)

	created := false
	if err := k8s.CopyFromPod(ctx, ap, abs, tmpPath); err != nil {
		log.Printf("file does not exist yet (%v); creating new", err)
		_ = os.WriteFile(tmpPath, []byte{}, 0644)
		created = true
	}

	before, err := fileChecksum(tmpPath)
	if err != nil {
		return err
	}

	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vi"
	}
	edit := exec.CommandContext(ctx, editor, tmpPath)
	edit.Stdin = os.Stdin
	edit.Stdout = os.Stdout
	edit.Stderr = os.Stderr
	if err := edit.Run(); err != nil {
		return fmt.Errorf("editor failed: %w", err)
	}

	after, err := fileChecksum(tmpPath)
	if err != nil {
		return err
	}
	if !created && before == after {
		fmt.Println("(unchanged)")
		return nil
	}
	return k8s.CopyToPod(ctx, ap, tmpPath, abs)
}

func fileChecksum(p string) (string, error) {
	b, err := os.ReadFile(p)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}
