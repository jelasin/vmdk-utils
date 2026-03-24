package state

import (
	"fmt"
	"os"
	"strings"

	"github.com/jelasin/vmdk-utils/internal/runtime"
)

type Health struct {
	DeviceExists          bool
	ImageExists           bool
	MountpointExists      bool
	MountpointMounted     bool
	PartitionDeviceExists bool
	Stale                 bool
	Reason                string
}

func (s Session) Health() Health {
	h := Health{}
	if s.Device != "" {
		_, h.DeviceExists = statOK(s.Device)
	}
	if s.ImagePath != "" {
		_, h.ImageExists = statOK(s.ImagePath)
	}
	if s.PartitionDevice != "" {
		_, h.PartitionDeviceExists = statOK(s.PartitionDevice)
	}
	if s.Mountpoint != "" {
		_, h.MountpointExists = statOK(s.Mountpoint)
		h.MountpointMounted = isMounted(s.Mountpoint)
	}

	h.Stale, h.Reason = evaluateStale(s, h)
	return h
}

func (s *Store) Cleanup(force bool) (removed int, kept int, err error) {
	remaining := make([]Session, 0, len(s.sessions))
	for _, session := range s.sessions {
		if force {
			removed++
			continue
		}
		health := session.Health()
		if health.Stale {
			removed++
			continue
		}
		remaining = append(remaining, session)
		kept++
	}
	s.sessions = remaining
	if err := s.save(); err != nil {
		return 0, 0, err
	}
	return removed, kept, nil
}

func statOK(path string) (os.FileInfo, bool) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, false
	}
	return info, true
}

func isMounted(path string) bool {
	output, err := runtime.RunCombined("mount")
	if err != nil {
		return false
	}
	needle := " on " + path + " "
	return strings.Contains(output, needle)
}

func evaluateStale(session Session, h Health) (bool, string) {
	if !h.ImageExists {
		return true, "image missing"
	}
	if session.Device != "" && !h.DeviceExists {
		return true, "device missing"
	}
	if session.Status == "mounted" {
		if session.Mountpoint == "" || !h.MountpointExists {
			return true, "mountpoint missing"
		}
		if !h.MountpointMounted {
			return true, "mountpoint not mounted"
		}
	}
	if session.PartitionDevice != "" && !h.PartitionDeviceExists {
		return true, "partition device missing"
	}
	return false, ""
}

func (h Health) String() string {
	if h.Stale {
		return fmt.Sprintf("stale(%s)", h.Reason)
	}
	return "ok"
}
