package qemu

import (
	"fmt"

	"github.com/jelasin/vmdk-utils/internal/runtime"
)

type ImageInfo struct {
	Human string
	JSON  string
}

func Inspect(image string) (ImageInfo, error) {
	jsonOutput, err := runtime.RunCombined("qemu-img", "info", "--output=json", image)
	if err != nil {
		return ImageInfo{}, fmt.Errorf("qemu-img info json: %w", err)
	}

	humanOutput, err := runtime.RunCombined("qemu-img", "info", image)
	if err != nil {
		return ImageInfo{}, fmt.Errorf("qemu-img info: %w", err)
	}

	return ImageInfo{Human: humanOutput, JSON: jsonOutput}, nil
}
