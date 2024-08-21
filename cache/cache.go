package cache

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/rs/zerolog/log"
)

type Cache struct {
	rootPath string
}

func New(rootPath string) (*Cache, error) {
	if _, err := os.Stat(rootPath); os.IsNotExist(err) {
		err := os.MkdirAll(rootPath, 0755)
		if err != nil {
			return &Cache{rootPath}, err
		}
	}
	log.Debug().Str("path", rootPath).Msg("Root path when creating Cache")
	return &Cache{rootPath}, nil
}

func (c *Cache) Download(url string, dir string, uniqueID string) (string, error) {

	ext, err := GetFileExtensionFromUrl(url)
	if err != nil {
		return "", err
	}
	err = os.MkdirAll(filepath.Join(c.rootPath, dir), 0755)
	if err != nil {
		return "", err
	}
	result := fmt.Sprintf("%s.%s", uniqueID, ext)
	out, err := os.Create(filepath.Join(c.rootPath, dir, fmt.Sprintf("%s.%s", uniqueID, ext)))
	if err != nil {
		return "", err
	}
	defer out.Close()

	// Get the data
	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return "", err
	}

	return result, nil
}

func GetFileExtensionFromUrl(rawUrl string) (string, error) {
	u, err := url.Parse(rawUrl)
	if err != nil {
		return "", err
	}
	pos := strings.LastIndex(u.Path, ".")
	if pos == -1 {
		return "", errors.New("couldn't find a period to indicate a file extension")
	}
	return u.Path[pos+1 : len(u.Path)], nil
}

func (c *Cache) createParent(path string) error {
	parent := filepath.Dir(path)
	if parent != "" {
		err := os.MkdirAll(filepath.Join(c.rootPath, parent), 0755)
		if err != nil {
			return err
		}
	}
	return nil

}
