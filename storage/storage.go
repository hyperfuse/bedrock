package storage

import (
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/rs/zerolog/log"
)

type Storage struct {
	rootPath string
}

func New(rootPath string) (*Storage, error) {
	if _, err := os.Stat(rootPath); os.IsNotExist(err) {
		err := os.MkdirAll(rootPath, 0755)
		if err != nil {
			return &Storage{rootPath}, err
		}
	}
	log.Debug().Str("path", rootPath).Msg("Root path when creating Cache")
	return &Storage{rootPath}, nil
}

func (s *Storage) Download(url string, to string) error {
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
