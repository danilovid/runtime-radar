package clickhouse

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/runtime-radar/runtime-radar/history-api/pkg/model"
	"gorm.io/gorm"
)

// admissionEventColumns is the set of columns to be selected from admission_events table.
// In fact, table contains many more columns, but they're mainly used for filtering.
const admissionEventColumns = "id,source_resource,threats,registered_at,kyverno_version,blocked,is_incident,incident_severity,block_by,notify_by"

type AdmissionEventRepository interface {
	Add(ctx context.Context, events *[]model.AdmissionEvent) error
	GetByID(ctx context.Context, id uuid.UUID) (*model.AdmissionEvent, error)
	GetRightSlice(ctx context.Context, cursor time.Time, filter any, sliceSize int) ([]*model.AdmissionEvent, error)
	GetLeftSlice(ctx context.Context, cursor time.Time, filter any, sliceSize int) ([]*model.AdmissionEvent, error)
}

type AdmissionEventDatabase struct {
	*gorm.DB
}

// Add adds events to the database. Unlike runtime events, admission events are produced at the rate
// of changes of the cluster and not at the rate of syscalls, so there is no batching database here.
func (d *AdmissionEventDatabase) Add(ctx context.Context, events *[]model.AdmissionEvent) error {
	if events == nil {
		return errors.New("nil pointer to events slice")
	}

	if len(*events) == 0 {
		return nil
	}

	return d.WithContext(ctx).Create(events).Error
}

func (d *AdmissionEventDatabase) GetByID(ctx context.Context, id uuid.UUID) (*model.AdmissionEvent, error) {
	if id == uuid.Nil {
		return nil, errors.New("incorrect admission event ID")
	}

	e := &model.AdmissionEvent{}

	err := d.WithContext(ctx).
		Select(admissionEventColumns).
		Where(&model.AdmissionEvent{ID: id}).
		Take(&e).
		Error

	return e, err
}

func (d *AdmissionEventDatabase) GetRightSlice(ctx context.Context, cursor time.Time, filter any, sliceSize int) ([]*model.AdmissionEvent, error) {
	var events []*model.AdmissionEvent

	if filter == nil {
		filter = ""
	}

	err := d.WithContext(ctx).
		Select(admissionEventColumns).
		Where(filter).
		// toDateTime64 has to be used because DateTime64 cannot be automatically converted from string. See https://clickhouse.com/docs/en/sql-reference/data-types/datetime64 for details.
		Where("registered_at < toDateTime64(?, 9, ?)", cursor.Format(DateTimeFormat), cursor.Location().String()).
		Order("registered_at DESC").
		Limit(sliceSize).
		Find(&events).
		Error

	return events, err
}

func (d *AdmissionEventDatabase) GetLeftSlice(ctx context.Context, cursor time.Time, filter any, sliceSize int) ([]*model.AdmissionEvent, error) {
	var events []*model.AdmissionEvent

	if filter == nil {
		filter = ""
	}

	err := d.WithContext(ctx).
		Select(admissionEventColumns).
		Where(filter).
		// toDateTime64 has to be used because DateTime64 cannot be automatically converted from string. See https://clickhouse.com/docs/en/sql-reference/data-types/datetime64 for details.
		Where("registered_at > toDateTime64(?, 9, ?)", cursor.Format(DateTimeFormat), cursor.Location().String()).
		Order("registered_at ASC").
		Limit(sliceSize).
		Find(&events).
		Error

	slices.Reverse(events)

	return events, err
}
