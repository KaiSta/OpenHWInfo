package shared

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// maxDRMCards bounds the scan for DRM card nodes. Real systems rarely expose
// more than a handful; the bound just keeps a malformed /sys from looping.
const maxDRMCards = 16

// DRMCardDevicePath returns the sysfs path of a DRM card's PCI device, e.g.
// "<sysDir>/class/drm/card0/device".
func DRMCardDevicePath(sysDir string, cardIndex int) string {
	return filepath.Join(sysDir, fmt.Sprintf("class/drm/card%d/device", cardIndex))
}

// ProbeDRMCards returns the indices of the DRM cards whose PCI vendor file
// matches vendorID (e.g. "0x1002" for AMD, "0x8086" for Intel). Cards that do
// not exist, or that belong to another vendor, are skipped.
func ProbeDRMCards(sysDir, vendorID string) []int {
	var indices []int
	for i := 0; i < maxDRMCards; i++ {
		devicePath := DRMCardDevicePath(sysDir, i)
		if _, err := os.Stat(devicePath); err != nil {
			continue
		}
		if vendor, ok := ReadSysFile(filepath.Join(devicePath, "vendor")); ok && vendor == vendorID {
			indices = append(indices, i)
		}
	}
	return indices
}

// ResolvePCIDevice reads the symlink at class/drm/cardN/device and returns the
// PCI address (e.g. "0000:0c:00.0"), or an empty string on error. A PCI-scoped
// lspci query with this address is the reliable way to name a specific card on a
// multi-GPU system.
func ResolvePCIDevice(sysDir string, cardIndex int) string {
	link, err := os.Readlink(DRMCardDevicePath(sysDir, cardIndex))
	if err != nil {
		return ""
	}
	parts := strings.Split(link, "/")
	return parts[len(parts)-1]
}
