package api

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"math"
	"net/http"
	"strconv"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/Anshuman115/heliolytics/internal/middleware"
	"github.com/Anshuman115/heliolytics/internal/store"
)

type profileHandler struct {
	update ProfileFunc
	now    func() time.Time
}

var profileFields = []string{"name", "birthDate", "sex", "heightCm", "weightKg"}

type profilePayload struct {
	name      *string
	birthDate *string
	sex       *string
	heightCm  *json.Number
	weightKg  *json.Number
}

func (h *profileHandler) prepare(r *http.Request, body []byte) (any, error) {
	fields, failure := strictObject(body)
	if failure != nil {
		return nil, failure
	}
	invalid := middleware.Fail(http.StatusBadRequest, middleware.CodeInvalidBody)
	if len(fields) != len(profileFields) {
		return nil, invalid
	}
	for _, name := range profileFields {
		if _, ok := fields[name]; !ok {
			return nil, invalid
		}
	}
	var payload profilePayload
	var ok bool
	if payload.name, ok = decodeNullableString(fields["name"]); !ok {
		return nil, invalid
	}
	if payload.birthDate, ok = decodeNullableString(fields["birthDate"]); !ok {
		return nil, invalid
	}
	if payload.sex, ok = decodeNullableString(fields["sex"]); !ok {
		return nil, invalid
	}
	if payload.heightCm, ok = decodeNullableNumber(fields["heightCm"]); !ok {
		return nil, invalid
	}
	if payload.weightKg, ok = decodeNullableNumber(fields["weightKg"]); !ok {
		return nil, invalid
	}
	return &payload, nil
}

func (h *profileHandler) put(w http.ResponseWriter, r *http.Request) {
	payload, ok := middleware.Payload(r).(*profilePayload)
	if !ok {
		middleware.WriteCode(w, http.StatusInternalServerError, middleware.CodeInternal)
		return
	}
	profile, failure := h.validate(payload)
	if failure != nil {
		middleware.WriteError(w, failure)
		return
	}
	if err := h.update(r.Context(), profile); err != nil {
		writeProfileError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *profileHandler) validate(payload *profilePayload) (*store.Profile, *middleware.Error) {
	invalid := middleware.Fail(http.StatusUnprocessableEntity, middleware.CodeInvalidProfile)
	profile := &store.Profile{}

	if payload.name != nil {
		if utf8.RuneCountInString(*payload.name) > 100 || hasControl(*payload.name) {
			return nil, invalid
		}
		name := *payload.name
		profile.Name = &name
	}
	if payload.birthDate != nil {
		birthDate, err := parseDay(*payload.birthDate)
		if err != nil {
			return nil, invalid
		}
		if !birthDate.After(store.Day(1900, time.January, 1)) || birthDate.After(istToday(h.now())) {
			return nil, invalid
		}
		profile.BirthDate = &birthDate
	}
	if payload.sex != nil {
		if *payload.sex != "male" && *payload.sex != "female" {
			return nil, invalid
		}
		sex := *payload.sex
		profile.Sex = &sex
	}
	if payload.heightCm != nil {
		height, ok := boundedNumber(*payload.heightCm, 50, 250)
		if !ok {
			return nil, invalid
		}
		profile.HeightCm = &height
	}
	if payload.weightKg != nil {
		weight, ok := boundedNumber(*payload.weightKg, 20, 300)
		if !ok {
			return nil, invalid
		}
		profile.WeightKg = &weight
	}
	return profile, nil
}

func boundedNumber(number json.Number, min, max float64) (float64, bool) {
	value, err := strconv.ParseFloat(string(number), 64)
	if err != nil || math.IsInf(value, 0) || math.IsNaN(value) {
		return 0, false
	}
	if value < min || value > max {
		return 0, false
	}
	return value, true
}

func hasControl(value string) bool {
	for _, r := range value {
		if unicode.IsControl(r) {
			return true
		}
	}
	return false
}

func writeProfileError(w http.ResponseWriter, err error) {
	var invalidRow *store.InvalidRowError
	switch {
	case errors.As(err, &invalidRow):
		middleware.WriteCode(w, http.StatusUnprocessableEntity, middleware.CodeInvalidProfile)
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		middleware.WriteCode(w, http.StatusServiceUnavailable, middleware.CodeRequestTimeout)
	default:
		log.Printf("profile update failed: %v", err)
		middleware.WriteCode(w, http.StatusInternalServerError, middleware.CodeInternal)
	}
}
