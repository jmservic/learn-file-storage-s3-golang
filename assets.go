package main

import (
	"os"
	"fmt"
	"path/filepath"
	"strings"
	"crypto/rand"
	"encoding/base64"
)

func (cfg apiConfig) ensureAssetsDir() error {
	if _, err := os.Stat(cfg.assetsRoot); os.IsNotExist(err) {
		return os.Mkdir(cfg.assetsRoot, 0755)
	}
	return nil
}

func getAssetPath(mediaType string) string {
	base := make([]byte, 32)
	_, err := rand.Read(base)
	if err != nil {
		panic("failed to generate random bytes")
	}
	fileID := base64.RawURLEncoding.EncodeToString(base)
	ext := mediaTypeToExt(mediaType)
	return fmt.Sprintf("%v%v", fileID, ext)
}

func (cfg apiConfig) getAssetDiskPath(assetPath string) string {
	return filepath.Join(cfg.assetsRoot, assetPath)
}

func (cfg apiConfig) getAssetURL(assetPath string) string {
	return fmt.Sprintf("http://localhost:%v/assets/%v", cfg.port, assetPath)

}

func mediaTypeToExt(mediaType string) string {
	parts := strings.Split(mediaType, "/")
	if len(parts) != 2 {
		return ".bin"
	}

	return "." + parts[1]
}
