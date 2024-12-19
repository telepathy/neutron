package service

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
)

func Extract(src io.ReadCloser, destDir string) error {
	gzipReader, err := gzip.NewReader(src)
	if err != nil {
		log.Fatal(err)
	}
	defer gzipReader.Close()
	tarReader := tar.NewReader(gzipReader)
	// 解压 tar 内容
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to read tar header: %v", err)
		}

		// 生成目标路径
		targetPath := filepath.Join(destDir, header.Name)

		// 防止路径穿越漏洞
		if !strings.HasPrefix(targetPath, filepath.Clean(destDir)+string(os.PathSeparator)) {
			return fmt.Errorf("illegal file path: %s", targetPath)
		}

		switch header.Typeflag {
		case tar.TypeDir:
			// 创建目录
			if err := os.MkdirAll(targetPath, os.FileMode(header.Mode)); err != nil {
				return fmt.Errorf("failed to create directory: %v", err)
			}
		case tar.TypeReg:
			// 创建文件
			if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
				return fmt.Errorf("failed to create parent directory: %v", err)
			}
			outFile, err := os.Create(targetPath)
			if err != nil {
				return fmt.Errorf("failed to create file: %v", err)
			}
			defer outFile.Close()

			// 写入文件内容
			if _, err := io.Copy(outFile, tarReader); err != nil {
				return fmt.Errorf("failed to write file content: %v", err)
			}
		}
	}
	return nil
}
