package k8s

import (
	"archive/tar"
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
)

// CopyToPod streams a single local file to remoteAbsPath inside the access pod.
// remoteAbsPath must be an absolute path (use AccessPod.AbsPath).
func CopyToPod(ctx context.Context, ap *AccessPod, localPath, remoteAbsPath string) error {
	info, err := os.Stat(localPath)
	if err != nil {
		return fmt.Errorf("stat local: %w", err)
	}
	if info.IsDir() {
		return fmt.Errorf("directory copy not supported")
	}

	remoteDir := path.Dir(remoteAbsPath)
	remoteName := path.Base(remoteAbsPath)
	if remoteName == "" || remoteName == "." || remoteName == "/" {
		return fmt.Errorf("invalid remote path %q", remoteAbsPath)
	}

	pr, pw := io.Pipe()
	defer pr.Close()

	writeErr := make(chan error, 1)
	go func() {
		err := func() error {
			f, err := os.Open(localPath)
			if err != nil {
				return err
			}
			defer f.Close()
			tw := tar.NewWriter(pw)
			hdr := &tar.Header{
				Name: remoteName,
				Mode: int64(info.Mode().Perm()),
				Size: info.Size(),
				Uid:  1000,
				Gid:  1000,
			}
			if err := tw.WriteHeader(hdr); err != nil {
				return err
			}
			if _, err := io.Copy(tw, f); err != nil {
				return err
			}
			return tw.Close()
		}()
		pw.CloseWithError(err)
		writeErr <- err
	}()

	cmd := []string{"sh", "-c", fmt.Sprintf("mkdir -p %s && tar -xf - -C %s",
		shellQuote(remoteDir), shellQuote(remoteDir))}
	execErr := ExecStream(ctx, ap.Namespace, ap.Name, ap.Container, cmd,
		ExecStreamOptions{Stdin: pr, Stdout: io.Discard, Stderr: os.Stderr})

	pr.Close() // unblock goroutine if exec died mid-write
	wErr := <-writeErr

	if execErr != nil {
		return execErr
	}
	return wErr
}

// BotDataExportExcludes are tar --exclude patterns applied when archiving a
// bot's data root for export — runtime-derivable junk we don't want in the
// archive.
var BotDataExportExcludes = []string{
	"node_modules",
	".cache",
	".npm",
	".openclaw-init-done",
}

// ArchiveFromPod streams a tar.gz of the bot's data root from the access pod
// to w. The archive contains a single top-level directory (the basename of
// ap.MountPath, e.g. ".openclaw"), so extraction yields a clean folder rather
// than scattering files into cwd.
//
// excludes are tar --exclude patterns (matched against names inside the
// archive); pass nil for none.
func ArchiveFromPod(ctx context.Context, ap *AccessPod, w io.Writer, excludes []string) error {
	parent := path.Dir(ap.MountPath)
	base := path.Base(ap.MountPath)

	var buf strings.Builder
	buf.WriteString("tar -cf - ")
	for _, e := range excludes {
		buf.WriteString("--exclude=")
		buf.WriteString(shellQuote(e))
		buf.WriteString(" ")
	}
	buf.WriteString("-C ")
	buf.WriteString(shellQuote(parent))
	buf.WriteString(" ")
	buf.WriteString(shellQuote(base))
	buf.WriteString(" | gzip -c")

	return ExecStream(ctx, ap.Namespace, ap.Name, ap.Container,
		[]string{"sh", "-c", buf.String()},
		ExecStreamOptions{Stdout: w, Stderr: os.Stderr})
}

// CopyFromPod streams a single file from the pod to localPath.
func CopyFromPod(ctx context.Context, ap *AccessPod, remoteAbsPath, localPath string) error {
	remoteDir := path.Dir(remoteAbsPath)
	remoteName := path.Base(remoteAbsPath)
	if remoteName == "" || remoteName == "." || remoteName == "/" {
		return fmt.Errorf("invalid remote path %q", remoteAbsPath)
	}

	pr, pw := io.Pipe()
	defer pr.Close()

	execErrCh := make(chan error, 1)
	go func() {
		err := ExecStream(ctx, ap.Namespace, ap.Name, ap.Container,
			[]string{"tar", "-cf", "-", "-C", remoteDir, remoteName},
			ExecStreamOptions{Stdout: pw, Stderr: os.Stderr})
		pw.CloseWithError(err)
		execErrCh <- err
	}()

	tr := tar.NewReader(pr)
	found := false
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			pr.Close()
			<-execErrCh
			return fmt.Errorf("tar read: %w", err)
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		if path.Base(hdr.Name) != remoteName {
			continue
		}
		f, err := os.OpenFile(localPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
		if err != nil {
			pr.Close()
			<-execErrCh
			return fmt.Errorf("open local: %w", err)
		}
		if _, err := io.Copy(f, tr); err != nil {
			f.Close()
			pr.Close()
			<-execErrCh
			return fmt.Errorf("write local: %w", err)
		}
		f.Close()
		found = true
		break
	}
	pr.Close() // ensure exec is unblocked / goroutine can finish
	execErr := <-execErrCh

	if !found {
		if execErr != nil {
			return fmt.Errorf("tar from pod: %w", execErr)
		}
		return fmt.Errorf("file %s not found", remoteAbsPath)
	}
	return nil
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
