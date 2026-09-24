package iot

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"ocm-backend/internal/dbutil"
	"ocm-backend/internal/logging"
)

var ErrNotFound = errors.New("iot device not found")

// deviceColumns must stay in sync with scanDevice.
const deviceColumns = "id, site, source_id, external_id, name, category, classroom_id, status, state, last_seen_at, created_at, updated_at"

// Store manages the iot_devices / iot_events / iot_commands tables.
type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store { return &Store{db: db} }

// Migrate creates the three IoT tables. It is idempotent and safe to run on
// every startup. iot_devices.classroom_id is a logical reference to
// classrooms.id (no FK, matching the repo convention); the unique identity
// key is the data plane's (site, source_id, external_id) triple. iot_events
// carries the dedup unique index that collapses QoS1 at-least-once
// redeliveries; occurred_at has millisecond precision because device event
// time is millisecond precision.
//
// Every temporal column declares its default explicitly. occurred_at and
// expires_at are DATETIME(3), not TIMESTAMP(3): a TIMESTAMP column that is
// neither the table's first TIMESTAMP nor declared NULL/DEFAULT is assigned
// an implicit '0000-00-00 00:00:00' default whenever the server runs with
// explicit_defaults_for_timestamp=OFF (which managed MySQL 8.0 offerings
// such as Tencent CynosDB do by default), and NO_ZERO_DATE in sql_mode then
// rejects the CREATE TABLE with Error 1067. DATETIME is outside that
// nonstandard behavior entirely, so these columns cannot break again — and
// DATETIME also drops the implicit ON UPDATE CURRENT_TIMESTAMP that a bare
// first TIMESTAMP column silently picks up, which would be wrong for an
// event time.
func (s *Store) Migrate(ctx context.Context) error {
	for _, stmt := range []string{
		`CREATE TABLE IF NOT EXISTS iot_devices (
		    id           BIGINT AUTO_INCREMENT PRIMARY KEY,
		    site         VARCHAR(32)  NOT NULL DEFAULT 'main',
		    source_id    VARCHAR(64)  NOT NULL,
		    external_id  VARCHAR(128) NOT NULL,
		    name         VARCHAR(128) NOT NULL DEFAULT '',
		    category     VARCHAR(32)  NOT NULL DEFAULT 'generic',
		    classroom_id BIGINT       NULL,
		    status       VARCHAR(16)  NOT NULL DEFAULT 'pending',
		    state        JSON         NULL,
		    last_seen_at TIMESTAMP(3) NULL,
		    created_at   TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
		    updated_at   TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
		    UNIQUE KEY uk_iot_device_identity (site, source_id, external_id),
		    KEY idx_iot_device_status (status),
		    KEY idx_iot_device_classroom (classroom_id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS iot_events (
		    id          BIGINT AUTO_INCREMENT PRIMARY KEY,
		    device_id   BIGINT      NOT NULL,
		    type        VARCHAR(64) NOT NULL,
		    payload     JSON        NULL,
		    occurred_at DATETIME(3) NOT NULL,
		    received_at TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
		    dedup_hash  CHAR(16)    NOT NULL,
		    UNIQUE KEY uk_iot_event_dedup (device_id, type, occurred_at, dedup_hash),
		    KEY idx_iot_event_device_time (device_id, occurred_at)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS iot_commands (
		    id         BIGINT AUTO_INCREMENT PRIMARY KEY,
		    command_id CHAR(32)    NOT NULL,
		    device_id  BIGINT      NOT NULL,
		    type       VARCHAR(64) NOT NULL,
		    payload    JSON        NULL,
		    status     VARCHAR(16) NOT NULL DEFAULT 'queued',
		    detail     VARCHAR(255) NOT NULL DEFAULT '',
		    issued_by  VARCHAR(64) NOT NULL DEFAULT '',
		    created_at TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
		    updated_at TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
		    expires_at DATETIME(3) NOT NULL,
		    UNIQUE KEY uk_iot_command_id (command_id),
		    KEY idx_iot_command_sweep (status, expires_at),
		    KEY idx_iot_command_device (device_id, created_at)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
	} {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("iot migrate: %w", err)
		}
	}
	return nil
}

func scanDevice(scan func(dest ...any) error) (Device, error) {
	var d Device
	var classroom sql.NullInt64
	var lastSeen sql.NullTime
	var state []byte
	if err := scan(&d.ID, &d.Site, &d.SourceID, &d.ExternalID, &d.Name, &d.Category,
		&classroom, &d.Status, &state, &lastSeen, &d.CreatedAt, &d.UpdatedAt); err != nil {
		return Device{}, err
	}
	if classroom.Valid {
		v := classroom.Int64
		d.ClassroomID = &v
	}
	if lastSeen.Valid {
		v := lastSeen.Time
		d.LastSeenAt = &v
	}
	if len(state) > 0 {
		d.State = json.RawMessage(state)
	}
	return d, nil
}

// PageDevices returns one page of devices matching the filters plus the total
// count. Pending (unclaimed) devices sort first so the claim queue is always
// on top of the console list.
func (s *Store) PageDevices(ctx context.Context, f DeviceFilter, q string, p dbutil.Pagination) ([]Device, int64, error) {
	where := ` FROM iot_devices WHERE 1=1`
	var args []any
	if f.Status != "" {
		where += ` AND status = ?`
		args = append(args, f.Status)
	}
	if f.SourceID != "" {
		where += ` AND source_id = ?`
		args = append(args, f.SourceID)
	}
	if f.Category != "" {
		where += ` AND category = ?`
		args = append(args, f.Category)
	}
	if f.ClassroomID > 0 {
		where += ` AND classroom_id = ?`
		args = append(args, f.ClassroomID)
	}
	if q != "" {
		where += ` AND (name LIKE ? OR external_id LIKE ?)`
		pat := dbutil.LikePattern(dbutil.EscapeLike(q))
		args = append(args, pat, pat)
	}
	query, queryArgs := p.AppendLimit(
		`SELECT `+deviceColumns+where+` ORDER BY status = 'pending' DESC, id`, args)
	rows, err := s.db.QueryContext(ctx, query, queryArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("page iot devices: %w", err)
	}
	defer func() { _ = rows.Close() }()

	devices := []Device{}
	for rows.Next() {
		d, err := scanDevice(rows.Scan)
		if err != nil {
			return nil, 0, fmt.Errorf("scan iot device: %w", err)
		}
		devices = append(devices, d)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate iot devices: %w", err)
	}
	total, err := dbutil.CountRows(ctx, s.db, where, args)
	if err != nil {
		return nil, 0, err
	}
	return devices, total, nil
}

func (s *Store) GetDeviceByID(ctx context.Context, id int64) (Device, error) {
	d, err := scanDevice(func(dest ...any) error {
		return s.db.QueryRowContext(ctx,
			`SELECT `+deviceColumns+` FROM iot_devices WHERE id = ?`, id).Scan(dest...)
	})
	if errors.Is(err, sql.ErrNoRows) {
		return Device{}, ErrNotFound
	}
	if err != nil {
		return Device{}, fmt.Errorf("get iot device by id: %w", err)
	}
	return d, nil
}

func (s *Store) deviceByIdentity(ctx context.Context, site, sourceID, externalID string) (Device, error) {
	d, err := scanDevice(func(dest ...any) error {
		return s.db.QueryRowContext(ctx,
			`SELECT `+deviceColumns+` FROM iot_devices WHERE site = ? AND source_id = ? AND external_id = ?`,
			site, sourceID, externalID).Scan(dest...)
	})
	if errors.Is(err, sql.ErrNoRows) {
		return Device{}, ErrNotFound
	}
	if err != nil {
		return Device{}, fmt.Errorf("get iot device by identity: %w", err)
	}
	return d, nil
}

// UpsertState stores the latest attribute object of a device and marks it
// seen. A first report auto-creates the row in pending status (the claim
// queue); categoryHint is the source-declared category applied only on that
// first insert — the operator can re-categorize later and later reports must
// not overwrite it. Pending devices stay pending regardless of traffic;
// approved devices flip online. The bool result reports whether the device
// was just created (used for the SSE device.created broadcast).
func (s *Store) UpsertState(ctx context.Context, site, sourceID, externalID, categoryHint string, state json.RawMessage, at time.Time) (Device, bool, error) {
	existing, err := s.deviceByIdentity(ctx, site, sourceID, externalID)
	if errors.Is(err, ErrNotFound) {
		category := categoryHint
		if category == "" {
			category = GenericCategory
		}
		_, ierr := s.db.ExecContext(ctx,
			`INSERT INTO iot_devices (site, source_id, external_id, category, status, state, last_seen_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?)`,
			site, sourceID, externalID, category, StatusPending, state, at)
		if ierr != nil {
			if dbutil.IsDuplicateEntry(ierr) {
				// Concurrent first report of the same identity — re-fetch and
				// update so this delivery is not lost.
				d, gerr := s.deviceByIdentity(ctx, site, sourceID, externalID)
				if gerr != nil {
					return Device{}, false, gerr
				}
				dev, uerr := s.updateState(ctx, d, state, at)
				return dev, false, uerr
			}
			return Device{}, false, fmt.Errorf("insert iot device: %w", ierr)
		}
		d, gerr := s.deviceByIdentity(ctx, site, sourceID, externalID)
		return d, true, gerr
	}
	if err != nil {
		return Device{}, false, err
	}
	d, uerr := s.updateState(ctx, existing, state, at)
	return d, false, uerr
}

func (s *Store) updateState(ctx context.Context, existing Device, state json.RawMessage, at time.Time) (Device, error) {
	// A pending (unclaimed) device must not flip to online just because it is
	// chatty — approval is the only path out of pending.
	if _, err := s.db.ExecContext(ctx,
		`UPDATE iot_devices SET state = ?, last_seen_at = ?,
		    status = IF(status = ?, status, ?)
		 WHERE id = ?`,
		state, at, StatusPending, StatusOnline, existing.ID); err != nil {
		return Device{}, fmt.Errorf("update iot device state: %w", err)
	}
	return s.GetDeviceByID(ctx, existing.ID)
}

// EnsureDevice guarantees a registry row exists for an identity (used by the
// event path so an alarm from an unknown device is recorded, never dropped).
func (s *Store) EnsureDevice(ctx context.Context, site, sourceID, externalID, categoryHint string) (Device, bool, error) {
	existing, err := s.deviceByIdentity(ctx, site, sourceID, externalID)
	if errors.Is(err, ErrNotFound) {
		d, _, uerr := s.UpsertState(ctx, site, sourceID, externalID, categoryHint, nil, time.Now())
		return d, true, uerr
	}
	if err != nil {
		return Device{}, false, err
	}
	return existing, false, nil
}

// ApproveDevice claims a pending device: names it, binds the classroom, and
// derives presence from last_seen (fresh report = online). An empty category
// keeps the source-declared one. Re-approving a claimed device applies the
// fields but leaves its status alone, so operators can fix mistakes without
// deleting history.
func (s *Store) ApproveDevice(ctx context.Context, id int64, in ApproveInput, onlineTTL time.Duration) (Device, error) {
	sets := []string{"name = ?", "classroom_id = ?"}
	args := []any{in.Name, in.ClassroomID}
	if in.Category != "" {
		sets = append(sets, "category = ?")
		args = append(args, in.Category)
	}
	// A pending (unclaimed) device must not flip to online just because its
	// last report is fresh — only the pending→approved transition derives
	// presence from last_seen; claimed devices keep their status here.
	sets = append(sets,
		`status = IF(status = ?, IF(last_seen_at IS NOT NULL AND last_seen_at >= NOW() - INTERVAL ? SECOND, ?, ?), status)`)
	args = append(args, StatusPending, int(onlineTTL.Seconds()), StatusOnline, StatusOffline, id)
	if _, err := s.db.ExecContext(ctx,
		`UPDATE iot_devices SET `+strings.Join(sets, ", ")+` WHERE id = ?`, args...); err != nil {
		return Device{}, fmt.Errorf("approve iot device: %w", err)
	}
	return s.GetDeviceByID(ctx, id)
}

// UpdateDevice applies the non-nil fields of a partial update. ClassroomID 0
// clears the binding (a device may not live in any classroom).
func (s *Store) UpdateDevice(ctx context.Context, id int64, in DeviceUpdateInput) (Device, error) {
	var sets []string
	var args []any
	if in.Name != nil {
		sets = append(sets, "name = ?")
		args = append(args, *in.Name)
	}
	if in.Category != nil {
		sets = append(sets, "category = ?")
		args = append(args, *in.Category)
	}
	if in.ClassroomID != nil {
		if *in.ClassroomID > 0 {
			sets = append(sets, "classroom_id = ?")
			args = append(args, *in.ClassroomID)
		} else {
			sets = append(sets, "classroom_id = NULL")
		}
	}
	if len(sets) == 0 {
		return Device{}, ErrNotFound
	}
	args = append(args, id)
	if _, err := s.db.ExecContext(ctx,
		`UPDATE iot_devices SET `+strings.Join(sets, ", ")+` WHERE id = ?`, args...); err != nil {
		return Device{}, fmt.Errorf("update iot device: %w", err)
	}
	return s.GetDeviceByID(ctx, id)
}

// DeleteDevice removes the registry row and its event/command history. The
// next report from the same identity re-creates a pending row — deletion is
// the "forget this device" action, not a blocklist.
func (s *Store) DeleteDevice(ctx context.Context, id int64) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM iot_events WHERE device_id = ?`, id); err != nil {
		return fmt.Errorf("delete iot events: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM iot_commands WHERE device_id = ?`, id); err != nil {
		return fmt.Errorf("delete iot commands: %w", err)
	}
	res, err := s.db.ExecContext(ctx, `DELETE FROM iot_devices WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete iot device: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete iot device rows affected: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// MarkSourceOffline flips every non-pending device of one source to offline
// (the source's MQTT will message fired — the gateway/controller or its
// uplink went away). Pending devices keep their status: they were never
// online.
func (s *Store) MarkSourceOffline(ctx context.Context, site, sourceID string) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		`UPDATE iot_devices SET status = ? WHERE site = ? AND source_id = ? AND status = ?`,
		StatusOffline, site, sourceID, StatusOnline)
	if err != nil {
		return 0, fmt.Errorf("mark iot source offline: %w", err)
	}
	return res.RowsAffected()
}

// SweepStaleDevices marks approved devices offline when no state message has
// arrived within ttl — the safety net for sources that vanish without a will
// message. Called from the retention loop.
func (s *Store) SweepStaleDevices(ctx context.Context, onlineTTL time.Duration) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		`UPDATE iot_devices SET status = ? WHERE status = ? AND last_seen_at < NOW() - INTERVAL ? SECOND`,
		StatusOffline, StatusOnline, int(onlineTTL.Seconds()))
	if err != nil {
		return 0, fmt.Errorf("sweep stale iot devices: %w", err)
	}
	return res.RowsAffected()
}

// EventDedupHash derives the dedup key fragment for an event: a stable digest
// of type + event time + payload. Two deliveries of the same event (QoS1
// at-least-once) collapse into one row via the dedup unique index.
func EventDedupHash(eventType string, occurredAt time.Time, payload []byte) string {
	h := sha256.New()
	h.Write([]byte(eventType))
	h.Write([]byte{0})
	h.Write([]byte(occurredAt.UTC().Format("2006-01-02T15:04:05.000Z07:00")))
	h.Write([]byte{0})
	h.Write(payload)
	return hex.EncodeToString(h.Sum(nil))[:16]
}

// InsertEvent records a device event. A duplicate (same device, type, event
// time and payload — i.e. a broker redelivery) is silently dropped and
// reported as (0, nil).
func (s *Store) InsertEvent(ctx context.Context, deviceID int64, eventType string, payload []byte, occurredAt time.Time) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO iot_events (device_id, type, payload, occurred_at, dedup_hash) VALUES (?, ?, ?, ?, ?)`,
		deviceID, eventType, payload, occurredAt, EventDedupHash(eventType, occurredAt, payload))
	if err != nil {
		if dbutil.IsDuplicateEntry(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("insert iot event: %w", err)
	}
	return res.LastInsertId()
}

func scanEvent(scan func(dest ...any) error) (DeviceEvent, error) {
	var e DeviceEvent
	var payload []byte
	if err := scan(&e.ID, &e.DeviceID, &e.Type, &payload, &e.OccurredAt, &e.ReceivedAt); err != nil {
		return DeviceEvent{}, err
	}
	if len(payload) > 0 {
		e.Payload = json.RawMessage(payload)
	}
	return e, nil
}

// PageEvents returns one page of a device's events, newest first.
func (s *Store) PageEvents(ctx context.Context, deviceID int64, p dbutil.Pagination) ([]DeviceEvent, int64, error) {
	where := ` FROM iot_events WHERE device_id = ?`
	query, queryArgs := p.AppendLimit(
		`SELECT id, device_id, type, payload, occurred_at, received_at`+where+` ORDER BY occurred_at DESC, id DESC`,
		[]any{deviceID})
	rows, err := s.db.QueryContext(ctx, query, queryArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("page iot events: %w", err)
	}
	defer func() { _ = rows.Close() }()

	events := []DeviceEvent{}
	for rows.Next() {
		e, err := scanEvent(rows.Scan)
		if err != nil {
			return nil, 0, fmt.Errorf("scan iot event: %w", err)
		}
		events = append(events, e)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate iot events: %w", err)
	}
	total, err := dbutil.CountRows(ctx, s.db, where, []any{deviceID})
	if err != nil {
		return nil, 0, err
	}
	return events, total, nil
}

func scanCommand(scan func(dest ...any) error) (DeviceCommand, error) {
	var c DeviceCommand
	var payload []byte
	if err := scan(&c.ID, &c.CommandID, &c.DeviceID, &c.Type, &payload, &c.Status,
		&c.Detail, &c.IssuedBy, &c.CreatedAt, &c.UpdatedAt, &c.ExpiresAt); err != nil {
		return DeviceCommand{}, err
	}
	if len(payload) > 0 {
		c.Payload = json.RawMessage(payload)
	}
	return c, nil
}

const commandColumns = "id, command_id, device_id, type, payload, status, detail, issued_by, created_at, updated_at, expires_at"

// newCommandID returns a 32-hex-char unique command identifier.
func newCommandID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand failing is a process-level emergency; panic surfaces it.
		panic(fmt.Errorf("iot: generate command id: %w", err))
	}
	return hex.EncodeToString(b)
}

// CreateCommand persists a queued command. The publisher flips it to
// delivered once the broker accepts the publish.
func (s *Store) CreateCommand(ctx context.Context, deviceID int64, cmdType string, payload []byte, issuedBy string, ttl time.Duration) (DeviceCommand, error) {
	commandID := newCommandID()
	expiresAt := time.Now().Add(ttl)
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO iot_commands (command_id, device_id, type, payload, status, issued_by, expires_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		commandID, deviceID, cmdType, payload, CmdStatusQueued, issuedBy, expiresAt); err != nil {
		return DeviceCommand{}, fmt.Errorf("create iot command: %w", err)
	}
	return s.GetCommandByID(ctx, commandID)
}

func (s *Store) GetCommandByID(ctx context.Context, commandID string) (DeviceCommand, error) {
	c, err := scanCommand(func(dest ...any) error {
		return s.db.QueryRowContext(ctx,
			`SELECT `+commandColumns+` FROM iot_commands WHERE command_id = ?`, commandID).Scan(dest...)
	})
	if errors.Is(err, sql.ErrNoRows) {
		return DeviceCommand{}, ErrNotFound
	}
	if err != nil {
		return DeviceCommand{}, fmt.Errorf("get iot command: %w", err)
	}
	return c, nil
}

// MarkCommandPublished transitions queued → delivered after the broker
// accepted the publish. Returns false when the command already left queued
// (expired or failed while publishing).
func (s *Store) MarkCommandPublished(ctx context.Context, commandID string) (bool, error) {
	return s.transitionCommand(ctx, commandID, []string{CmdStatusQueued}, CmdStatusDelivered, "")
}

// FailCommand records a publish-side failure.
func (s *Store) FailCommand(ctx context.Context, commandID, detail string) error {
	_, err := s.transitionCommand(ctx, commandID,
		[]string{CmdStatusQueued, CmdStatusDelivered}, CmdStatusFailed, detail)
	return err
}

// ApplyAck applies a device-reported outcome (acked/failed) to a live
// command. Returns false when the command is unknown, already settled, or
// expired — late acks must never resurrect a settled row.
func (s *Store) ApplyAck(ctx context.Context, commandID, status, detail string) (bool, error) {
	return s.transitionCommand(ctx, commandID,
		[]string{CmdStatusQueued, CmdStatusDelivered}, status, detail)
}

func (s *Store) transitionCommand(ctx context.Context, commandID string, from []string, to, detail string) (bool, error) {
	in := "('" + strings.Join(from, "','") + "')"
	res, err := s.db.ExecContext(ctx,
		`UPDATE iot_commands SET status = ?, detail = ? WHERE command_id = ? AND status IN `+in,
		to, detail, commandID)
	if err != nil {
		return false, fmt.Errorf("transition iot command: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("transition iot command rows affected: %w", err)
	}
	return n > 0, nil
}

// ExpireCommands settles queued/delivered commands whose deadline passed, so
// a command queued during an outage can never fire on reconnect.
func (s *Store) ExpireCommands(ctx context.Context) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		`UPDATE iot_commands SET status = ?, detail = ? WHERE status IN (?, ?) AND expires_at <= NOW(3)`,
		CmdStatusExpired, "expired before delivery/ack", CmdStatusQueued, CmdStatusDelivered)
	if err != nil {
		return 0, fmt.Errorf("expire iot commands: %w", err)
	}
	return res.RowsAffected()
}

// PageCommands returns one page of commands, newest first, optionally
// filtered by status and device.
func (s *Store) PageCommands(ctx context.Context, f CommandFilter, p dbutil.Pagination) ([]DeviceCommand, int64, error) {
	where := ` FROM iot_commands WHERE 1=1`
	var args []any
	if f.Status != "" {
		where += ` AND status = ?`
		args = append(args, f.Status)
	}
	if f.DeviceID > 0 {
		where += ` AND device_id = ?`
		args = append(args, f.DeviceID)
	}
	query, queryArgs := p.AppendLimit(
		`SELECT `+commandColumns+where+` ORDER BY created_at DESC, id DESC`, args)
	rows, err := s.db.QueryContext(ctx, query, queryArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("page iot commands: %w", err)
	}
	defer func() { _ = rows.Close() }()

	cmds := []DeviceCommand{}
	for rows.Next() {
		c, err := scanCommand(rows.Scan)
		if err != nil {
			return nil, 0, fmt.Errorf("scan iot command: %w", err)
		}
		cmds = append(cmds, c)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate iot commands: %w", err)
	}
	total, err := dbutil.CountRows(ctx, s.db, where, args)
	if err != nil {
		return nil, 0, err
	}
	return cmds, total, nil
}

// PurgeExpired deletes events and commands older than the retention window
// and returns the total deleted count.
func (s *Store) PurgeExpired(ctx context.Context, retentionDays int) (int64, error) {
	var total int64
	for _, stmt := range []string{
		`DELETE FROM iot_events WHERE received_at < NOW() - INTERVAL ? DAY`,
		`DELETE FROM iot_commands WHERE created_at < NOW() - INTERVAL ? DAY`,
	} {
		res, err := s.db.ExecContext(ctx, stmt, retentionDays)
		if err != nil {
			return total, fmt.Errorf("purge iot rows: %w", err)
		}
		if n, err := res.RowsAffected(); err == nil {
			total += n
		}
	}
	return total, nil
}

// RunPresenceLoop expires overdue commands and flips approved devices whose
// last report is older than onlineTTL to offline — the safety net for sources
// that vanish without a will message. Runs at a short interval (presence is
// near-real-time state, unlike retention).
func (s *Store) RunPresenceLoop(ctx context.Context, interval time.Duration, onlineTTL time.Duration) {
	pass := func() {
		if n, err := s.ExpireCommands(ctx); err != nil {
			logging.L.Error("iot: expire commands failed", "err", err)
		} else if n > 0 {
			logging.L.Info("iot: expired commands", "count", n)
		}
		if n, err := s.SweepStaleDevices(ctx, onlineTTL); err != nil {
			logging.L.Error("iot: stale device sweep failed", "err", err)
		} else if n > 0 {
			logging.L.Info("iot: swept stale devices offline", "count", n)
		}
	}
	pass()
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			pass()
		}
	}
}

// RunRetentionLoop purges once and then every interval until ctx is done
// (systemlog.RunRetentionLoop precedent), deleting events and commands older
// than the retention window.
func (s *Store) RunRetentionLoop(ctx context.Context, interval time.Duration, retentionDays int) {
	pass := func() {
		if n, err := s.PurgeExpired(ctx, retentionDays); err != nil {
			logging.L.Error("iot: retention purge failed", "err", err)
		} else if n > 0 {
			logging.L.Info("iot: retention purge deleted rows", "count", n)
		}
	}
	pass()
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			pass()
		}
	}
}
