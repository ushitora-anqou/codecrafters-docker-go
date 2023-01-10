package main

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path"
	"strconv"
	"strings"
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

func easyGet(url, token string) ([]byte, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	if token != "" {
		req.Header.Add("Authorization", "Bearer "+token)
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return body, nil
}

func fetchImage(image, outdir string) error {
	if !strings.Contains(image, "/") {
		image = "library/" + image
	}
	colonIndex := strings.LastIndex(image, ":")
	var imageName, imageRef string
	if colonIndex != -1 {
		imageName = image[0:colonIndex]
		imageRef = image[colonIndex+1:]
	}

	// Get token
	authBody, err := easyGet("https://auth.docker.io/token?service=registry.docker.io&scope=repository:"+imageName+":pull", "")
	if err != nil {
		return err
	}
	authJson := make(map[string]interface{})
	if err := json.Unmarshal(authBody, &authJson); err != nil {
		return err
	}
	token := authJson["token"].(string)

	// Get manifest
	maniBody, err := easyGet("https://registry.hub.docker.com/v2/"+imageName+"/manifests/"+imageRef, token)
	if err != nil {
		return err
	}
	maniJson := make(map[string]interface{})
	if err := json.Unmarshal(maniBody, &maniJson); err != nil {
		return err
	}
	fsLayers := []string{}
	for _, l := range maniJson["fsLayers"].([]interface{}) {
		blobSum := l.(map[string]interface{})["blobSum"].(string)
		fsLayers = append(fsLayers, blobSum)
	}

	// Get blob under tempdir
	tempdir, err := os.MkdirTemp("", "codecrafters-docker-blobs-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tempdir)
	for i, blobSum := range fsLayers {
		blobBody, err := easyGet("https://registry.hub.docker.com/v2/library/ubuntu/blobs/"+blobSum, token)
		if err != nil {
			return err
		}

		w, err := os.Create(path.Join(tempdir, strconv.Itoa(i)))
		if err != nil {
			return err
		}
		if _, err := w.Write(blobBody); err != nil {
			return err
		}
		w.Close()
	}

	// Extract blobs under outdir
	for i := len(fsLayers) - 1; i >= 0; i-- {
		cmd := exec.Command("tar", "xf", path.Join(tempdir, strconv.Itoa(i)), "-C", outdir)
		if err := cmd.Run(); err != nil {
			return err
		}
	}

	return nil
}

// Usage: your_docker.sh run <image> <command> <arg1> <arg2> ...
func main() {
	image := os.Args[2]
	command := os.Args[3]
	args := os.Args[4:len(os.Args)]

	tempdir, err := os.MkdirTemp("", "codecrafters-docker-*")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(tempdir)

	// Fetch image from DockerHub
	if err := fetchImage(image, tempdir); err != nil {
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
