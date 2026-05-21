package launcher

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// 平台 → 下载文件名映射
var platformArchive = map[string]string{
	"darwin_arm64":  "obscura-aarch64-macos.tar.gz",
	"darwin_amd64":  "obscura-x86_64-macos.tar.gz",
	"linux_amd64":   "obscura-x86_64-linux.tar.gz",
	"linux_arm64":   "obscura-aarch64-linux.tar.gz",
	"windows_amd64": "obscura-x86_64-windows.zip",
}

// binName 返回当前平台的 obscura 二进制名。
func binName() string {
	if runtime.GOOS == "windows" {
		return "obscura.exe"
	}
	return "obscura"
}

// defaultCacheDir 返回默认缓存目录。
func defaultCacheDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".cache", "obscura-go")
}

// Browser 管理 obscura 二进制的下载。
type Browser struct {
	Version    string // "latest" 或具体版本号
	CacheDir   string
	HTTPClient *http.Client
}

// NewBrowser 创建默认 Browser 实例。
func NewBrowser() *Browser {
	return &Browser{
		Version:    "latest",
		CacheDir:   defaultCacheDir(),
		HTTPClient: http.DefaultClient,
	}
}

// BinPath 返回当前平台的 obscura 二进制路径。
func (b *Browser) BinPath() string {
	return filepath.Join(b.CacheDir, b.Version, binName())
}

// Get 确保二进制存在，不存在则下载。
func (b *Browser) Get(ctx context.Context) (string, error) {
	binPath := b.BinPath()

	if b.validate(binPath) == nil {
		return binPath, nil
	}

	// 清理可能损坏的缓存
	_ = os.RemoveAll(filepath.Join(b.CacheDir, b.Version))

	archive, ok := platformArchive[runtime.GOOS+"_"+runtime.GOARCH]
	if !ok {
		return "", fmt.Errorf("launcher: 不支持的平台: %s/%s", runtime.GOOS, runtime.GOARCH)
	}

	url := fmt.Sprintf(
		"https://github.com/h4ckf0r0day/obscura/releases/%s/download/%s",
		b.Version, archive,
	)
	if b.Version == "latest" {
		url = fmt.Sprintf(
			"https://github.com/h4ckf0r0day/obscura/releases/latest/download/%s",
			archive,
		)
	}

	if err := b.download(ctx, url, archive); err != nil {
		return "", fmt.Errorf("launcher: 下载失败: %w", err)
	}

	if err := b.validate(binPath); err != nil {
		return "", fmt.Errorf("launcher: 验证失败: %w", err)
	}

	return binPath, nil
}

func (b *Browser) validate(binPath string) error {
	_, err := os.Stat(binPath)
	if err != nil {
		return err
	}
	cmd := exec.Command(binPath, "--version")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("无法执行 obscura: %w", err)
	}
	return nil
}

func (b *Browser) download(ctx context.Context, url, archive string) error {
	dir := filepath.Join(b.CacheDir, b.Version)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}

	resp, err := b.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, url)
	}

	tmpFile, err := os.CreateTemp("", "obscura-*"+filepath.Ext(archive))
	if err != nil {
		return err
	}
	defer os.Remove(tmpFile.Name())

	if _, err := io.Copy(tmpFile, resp.Body); err != nil {
		tmpFile.Close()
		return err
	}
	tmpFile.Close()

	switch {
	case filepath.Ext(archive) == ".gz" || filepath.Ext(archive) == ".tgz":
		return extractTarGz(tmpFile.Name(), dir)
	case filepath.Ext(archive) == ".zip":
		return extractZip(tmpFile.Name(), dir)
	default:
		return fmt.Errorf("不支持的文件格式: %s", archive)
	}
}

func extractTarGz(path, dest string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		target := filepath.Join(dest, filepath.Base(hdr.Name))
		if hdr.Typeflag == tar.TypeDir {
			os.MkdirAll(target, 0755)
			continue
		}

		out, err := os.Create(target)
		if err != nil {
			return err
		}
		if _, err := io.Copy(out, tr); err != nil {
			out.Close()
			return err
		}
		out.Close()

		if err := os.Chmod(target, 0755); err != nil {
			return err
		}
	}
	return nil
}

func extractZip(path, dest string) error {
	r, err := zip.OpenReader(path)
	if err != nil {
		return err
	}
	defer r.Close()

	for _, f := range r.File {
		target := filepath.Join(dest, filepath.Base(f.Name))
		if f.FileInfo().IsDir() {
			os.MkdirAll(target, 0755)
			continue
		}

		rc, err := f.Open()
		if err != nil {
			return err
		}

		out, err := os.Create(target)
		if err != nil {
			rc.Close()
			return err
		}
		_, err = io.Copy(out, rc)
		rc.Close()
		out.Close()
		if err != nil {
			return err
		}
	}
	return nil
}
