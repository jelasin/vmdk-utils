package commands

import (
	"github.com/jelasin/vmdk-utils/internal/nbd"
	"github.com/jelasin/vmdk-utils/internal/state"
)

func ensureSession(store *state.Store, image, requestedDevice string, readOnly bool) (state.Session, bool, error) {
	if session, ok := store.FindByImage(image); ok {
		return session, false, nil
	}

	device := requestedDevice
	var err error
	if device == "" {
		device, err = nbd.FindFreeDevice()
		if err != nil {
			return state.Session{}, false, err
		}
	}

	if err := nbd.Attach(image, device, readOnly); err != nil {
		return state.Session{}, false, err
	}

	session := state.Session{
		ImagePath: image,
		Device:    device,
		ReadOnly:  readOnly,
		Status:    "attached",
	}
	if err := store.Upsert(session); err != nil {
		_ = nbd.Detach(device)
		return state.Session{}, false, err
	}

	return session, true, nil
}
