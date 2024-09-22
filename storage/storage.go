package storage

import (
	"io"
	"net/http"
	"os"
	"path/filepath"

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

func (c *Cache) Download(url string, to string) error {
	parent := filepath.Dir(to)
	if parent != "" {
		err := os.MkdirAll(filepath.Join(c.rootPath, parent), 0755)
		if err != nil {
			return err
		}
	}
	fp := filepath.Join(c.rootPath, to)
	err := os.MkdirAll(filepath.Join(c.rootPath, parent), 0755)
	if err != nil {
		return err
	}

	out, err := os.Create(fp)
	if err != nil {
		return err
	}
	defer out.Close()

	// Get the data
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return err
	}

	return nil
}
