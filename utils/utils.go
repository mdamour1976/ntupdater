package utils

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

func FileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil || !os.IsNotExist(err)
}

func DownloadFile(url, filepath string) error {
	response, err := http.Get(url)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return err
	}

	out, err := os.Create(filepath)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, response.Body)
	if err != nil {
		return err
	}
	return nil
}

func DownloadAndExtract(version string) error {
	// check if local binary exists for remote archive
	if FileExists("./updates/" + version) {
		return nil
	}

	archiveName := "worker-" + version + "-" + runtime.GOOS + "-" + runtime.GOARCH
	if runtime.GOOS == "windows" {
		archiveName += ".zip"
	} else {
		archiveName += ".tar.gz"
	}
	os.MkdirAll("updates/"+version, 0755)
	downloadURL := "https://github.com/mdamour1976/ntworker/releases/download/" + version + "/" + archiveName
	downloadPath := "./updates/" + version + "/" + archiveName
	// download appropriate version if needed
	err := DownloadFile(downloadURL, downloadPath)
	if err != nil {
		return err
	}
	// extract archive to version specific folder
	err = ExtractArchive(downloadPath, "./updates/"+version+"/")
	if err != nil {
		return err
	}
	// remove archive
	defer func() {
		err = os.Remove(downloadPath)
		if err != nil {
			log.Println(err)
		}
	}()
	log.Println("Pulled version: " + version)
	return nil
}

func ExtractArchive(archivePath, destination string) error {
	if strings.HasSuffix(archivePath, ".zip") {
		reader, err := zip.OpenReader(archivePath)
		if err != nil {
			return err
		}
		defer reader.Close()

		for _, f := range reader.File {
			fpath := filepath.Join(destination, f.Name)
			if !strings.HasPrefix(fpath, filepath.Clean(destination)+string(os.PathSeparator)) {
				return nil
			}
			if f.FileInfo().IsDir() {
				os.MkdirAll(fpath, os.ModePerm)
				continue
			}
			os.MkdirAll(filepath.Dir(fpath), os.ModePerm)
			inFile, _ := f.Open()
			outFile, _ := os.Create(fpath)
			io.Copy(outFile, inFile)
			defer func() {
				inFile.Close()
				outFile.Close()
			}()
		}
	} else {
		// tar ball
		f, err := os.Open(archivePath)
		if err != nil {
			return err
		}
		defer f.Close()

		gzr, err := gzip.NewReader(f)
		if err != nil {
			return err
		}
		defer gzr.Close()

		tr := tar.NewReader(gzr)
		for {
			tarHeader, err := tr.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				return err
			}
			path := filepath.Join(destination, tarHeader.Name)
			if tarHeader.FileInfo().IsDir() {
				if err := os.MkdirAll(path, tarHeader.FileInfo().Mode()); err != nil {
					return err
				}
				continue
			}
			if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
				return err
			}
			out, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|os.O_TRUNC, tarHeader.FileInfo().Mode())
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				return err
			}
			out.Close()
		}
	}
	return nil
}
