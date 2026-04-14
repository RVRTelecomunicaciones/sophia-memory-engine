package shared

import (
	"crypto/rand"
	"time"

	"github.com/oklog/ulid/v2"
)

// RecordID is a ULID-based identifier for domain entities.
type RecordID struct {
	value ulid.ULID
}

// NewRecordID generates a new RecordID with the current timestamp.
func NewRecordID() RecordID {
	return RecordID{value: ulid.MustNew(ulid.Timestamp(time.Now()), rand.Reader)}
}

// RecordIDFromString parses a ULID string into a RecordID.
func RecordIDFromString(s string) (RecordID, error) {
	parsed, err := ulid.Parse(s)
	if err != nil {
		return RecordID{}, NewValidationError(FieldError{
			Field:   "record_id",
			Message: "invalid ULID format",
			Value:   s,
		})
	}
	return RecordID{value: parsed}, nil
}

// String returns the canonical string representation of the RecordID.
func (r RecordID) String() string {
	return r.value.String()
}

// IsValid returns true if the RecordID contains a valid non-zero ULID.
func (r RecordID) IsValid() bool {
	return r.value.Compare(ulid.ULID{}) != 0
}

// IsZero returns true if the RecordID is the zero value.
func (r RecordID) IsZero() bool {
	return r.value.Compare(ulid.ULID{}) == 0
}

// Time returns the timestamp encoded in the ULID.
func (r RecordID) Time() time.Time {
	return ulid.Time(r.value.Time())
}

// Equal returns true if both RecordIDs have the same value.
func (r RecordID) Equal(other RecordID) bool {
	return r.value.Compare(other.value) == 0
}
