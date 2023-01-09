package main

import (
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

func copyFile(srcFpath, dstFpath string) error {
	src, err := os.Open(srcFpath)
	if err != nil {
		return err
	}
	defer src.Close()

	srcInfo, err := src.Stat()
	if err != nil {
		return err
	}

	dst, err := os.OpenFile(dstFpath, os.O_CREATE|os.O_RDWR|os.O_TRUNC, srcInfo.Mode().Perm())
	if err != nil {
		return err
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		return err
	}

	return nil
}

// Usage: your_docker.sh run <image> <command> <arg1> <arg2> ...
func main() {
	command := os.Args[3]
	args := os.Args[4:len(os.Args)]

	tempdir, err := os.MkdirTemp("", "codecrafters-docker-*")
	if err != nil {
		log.Fatal(err)
	}
	//defer os.RemoveAll(tempdir)

	commandTempPath := filepath.Join(tempdir, command)
	commandTempDir := filepath.Dir(commandTempPath)
	if err := os.MkdirAll(commandTempDir, 0755); err != nil {
		log.Fatal(err)
	}
	if err := copyFile(command, filepath.Join(tempdir, command)); err != nil {
		log.Fatal(err)
	}

	// Chdir & Chroot
	if err := os.Chdir(tempdir); err != nil {
		log.Fatal(err)
	}
	if err := syscall.Chroot(tempdir); err != nil {
		log.Fatal(err)
	}

	// Unshare
	if err := syscall.Unshare(syscall.CLONE_NEWPID); err != nil {
		log.Fatal(err)
	}

	cmd := exec.Command(command, args...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		log.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		log.Fatal(err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		log.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		log.Fatal(err)
	}
	if err := stdin.Close(); err != nil {
		log.Fatal(err)
	}
	io.Copy(os.Stdout, stdout)
	io.Copy(os.Stderr, stderr)

	cmd.Wait()
	os.Exit(cmd.ProcessState.ExitCode())
}
