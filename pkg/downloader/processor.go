// Modified by the xiaohongshu-mcp-chatgpt maintainers; see NOTICE for derivative changes.
package downloader

import (
	"fmt"
	"os"

	"github.com/xpzouying/xiaohongshu-mcp/configs"
)

// ImageProcessor 图片处理器
type ImageProcessor struct {
	downloader *ImageDownloader
}

// NewImageProcessor 创建图片处理器
func NewImageProcessor() *ImageProcessor {
	return &ImageProcessor{
		downloader: NewImageDownloader(configs.GetImagesPath()),
	}
}

// ProcessImages 处理图片列表，返回本地文件路径
// 支持两种输入格式：
// 1. URL格式 (http/https开头) - 自动下载到本地
// 2. 本地文件路径 - 直接使用
// 保持原始图片顺序，如果下载失败直接返回错误
func (p *ImageProcessor) ProcessImages(images []string) ([]string, error) {
	localPaths := make([]string, 0, len(images))
	if err := ValidateLocalImages(images); err != nil {
		return nil, err
	}

	// 按顺序处理每张图片
	for _, image := range images {
		if IsImageURL(image) {
			// URL图片：立即下载，失败直接返回错误
			localPath, err := p.downloader.DownloadImage(image)
			if err != nil {
				return nil, fmt.Errorf("下载图片失败 %s: %w", image, err)
			}
			localPaths = append(localPaths, localPath)
		} else {
			// 本地路径已在循环前完整校验，避免部分图片失败后仍继续发布。
			localPaths = append(localPaths, image)
		}
	}

	if len(localPaths) == 0 {
		return nil, fmt.Errorf("no valid images found")
	}

	return localPaths, nil
}

// ValidateLocalImages 在任何网络下载或浏览器操作前，验证全部本地图片路径。
func ValidateLocalImages(images []string) error {
	for _, path := range images {
		if IsImageURL(path) {
			continue
		}
		info, err := os.Stat(path)
		if err != nil {
			return fmt.Errorf("本地图片路径不可访问 %q: %w", path, err)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("本地图片路径不是普通文件: %q", path)
		}
		file, err := os.Open(path)
		if err != nil {
			return fmt.Errorf("本地图片路径不可读 %q: %w", path, err)
		}
		if err := file.Close(); err != nil {
			return fmt.Errorf("关闭本地图片文件失败 %q: %w", path, err)
		}
	}
	return nil
}
