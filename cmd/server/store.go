package main

import (
	"database/sql"
	"errors"
	"time"

	_ "modernc.org/sqlite"
)

type Client struct {
	ID                   string    `json:"id"`
	Name                 string    `json:"name"`
	Segment              string    `json:"segment"`
	City                 string    `json:"city"`
	Version              string    `json:"version"`
	LastBackupDate       time.Time `json:"last_backup_date"`
	BackupSizeMB         float64   `json:"backup_size_mb"`
	PreviousBackupSizeMB float64   `json:"previous_backup_size_mb"`
	BackupIntegrityOK    bool      `json:"backup_integrity_ok"`
	BackupChecksum       string    `json:"backup_checksum"`
	Sales7d              float64   `json:"sales_last_7d"`
	SalesAvgWeekly       float64   `json:"sales_avg_weekly"`
	Tickets30d           int       `json:"tickets_last_30d"`
	DaysSinceInteraction int       `json:"days_since_last_interaction"`
	UpdatedAt            time.Time `json:"updated_at"`
}

type Ticket struct {
	ID        int64     `json:"id"`
	ClientID  string    `json:"client_id"`
	Note      string    `json:"note"`
	CreatedAt time.Time `json:"created_at"`
}

type Store struct{ db *sql.DB }

var ErrNotFound = errors.New("client not found")

func OpenStore(path string) (*Store, error) {
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1) // serializes writes; plenty for this workload
	const schema = `
CREATE TABLE IF NOT EXISTS clients (
	id TEXT PRIMARY KEY, name TEXT NOT NULL, segment TEXT NOT NULL, city TEXT NOT NULL,
	version TEXT NOT NULL, last_backup_date INTEGER NOT NULL,
	backup_size_mb REAL NOT NULL, previous_backup_size_mb REAL NOT NULL,
	backup_integrity_ok INTEGER NOT NULL, backup_checksum TEXT NOT NULL,
	sales_last_7d REAL NOT NULL, sales_avg_weekly REAL NOT NULL,
	tickets_last_30d INTEGER NOT NULL, days_since_last_interaction INTEGER NOT NULL,
	updated_at INTEGER NOT NULL);
CREATE TABLE IF NOT EXISTS tickets (
	id INTEGER PRIMARY KEY AUTOINCREMENT, client_id TEXT NOT NULL, note TEXT NOT NULL,
	status TEXT NOT NULL DEFAULT 'open', created_at INTEGER NOT NULL);`
	if _, err := db.Exec(schema); err != nil {
		return nil, err
	}
	return &Store{db: db}, nil
}

const cols = `id,name,segment,city,version,last_backup_date,backup_size_mb,previous_backup_size_mb,
backup_integrity_ok,backup_checksum,sales_last_7d,sales_avg_weekly,tickets_last_30d,days_since_last_interaction,updated_at`

type scanner interface{ Scan(...any) error }

func scanClient(s scanner) (Client, error) {
	var c Client
	var lb, up int64
	var ok int
	err := s.Scan(&c.ID, &c.Name, &c.Segment, &c.City, &c.Version, &lb, &c.BackupSizeMB, &c.PreviousBackupSizeMB,
		&ok, &c.BackupChecksum, &c.Sales7d, &c.SalesAvgWeekly, &c.Tickets30d, &c.DaysSinceInteraction, &up)
	c.LastBackupDate = time.Unix(lb, 0).UTC()
	c.UpdatedAt = time.Unix(up, 0).UTC()
	c.BackupIntegrityOK = ok == 1
	return c, err
}

func (s *Store) Count() (n int, err error) {
	err = s.db.QueryRow(`SELECT COUNT(*) FROM clients`).Scan(&n)
	return
}

func (s *Store) List() ([]Client, error) {
	rows, err := s.db.Query(`SELECT ` + cols + ` FROM clients`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Client
	for rows.Next() {
		c, err := scanClient(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) Get(id string) (Client, error) {
	c, err := scanClient(s.db.QueryRow(`SELECT `+cols+` FROM clients WHERE id=?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return c, ErrNotFound
	}
	return c, err
}

func (s *Store) Save(c Client) error {
	b2i := 0
	if c.BackupIntegrityOK {
		b2i = 1
	}
	_, err := s.db.Exec(`INSERT INTO clients (`+cols+`) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT(id) DO UPDATE SET name=excluded.name, segment=excluded.segment, city=excluded.city, version=excluded.version,
last_backup_date=excluded.last_backup_date, backup_size_mb=excluded.backup_size_mb,
previous_backup_size_mb=excluded.previous_backup_size_mb, backup_integrity_ok=excluded.backup_integrity_ok,
backup_checksum=excluded.backup_checksum, sales_last_7d=excluded.sales_last_7d, sales_avg_weekly=excluded.sales_avg_weekly,
tickets_last_30d=excluded.tickets_last_30d, days_since_last_interaction=excluded.days_since_last_interaction,
updated_at=excluded.updated_at`,
		c.ID, c.Name, c.Segment, c.City, c.Version, c.LastBackupDate.Unix(), c.BackupSizeMB, c.PreviousBackupSizeMB,
		b2i, c.BackupChecksum, c.Sales7d, c.SalesAvgWeekly, c.Tickets30d, c.DaysSinceInteraction, c.UpdatedAt.Unix())
	return err
}

func (s *Store) OpenTickets() (map[string]Ticket, error) {
	rows, err := s.db.Query(`SELECT id, client_id, note, created_at FROM tickets WHERE status='open' ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	m := map[string]Ticket{}
	for rows.Next() {
		var t Ticket
		var ts int64
		if err := rows.Scan(&t.ID, &t.ClientID, &t.Note, &ts); err != nil {
			return nil, err
		}
		t.CreatedAt = time.Unix(ts, 0).UTC()
		m[t.ClientID] = t
	}
	return m, rows.Err()
}

// CancelTicket closes the client's open ticket; reports whether one existed.
func (s *Store) CancelTicket(clientID string) (bool, error) {
	res, err := s.db.Exec(`UPDATE tickets SET status='cancelled' WHERE client_id=? AND status='open'`, clientID)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// OpenTicket returns the existing open ticket or creates a new one.
func (s *Store) OpenTicket(clientID, note string) (Ticket, bool, error) {
	if m, err := s.OpenTickets(); err != nil {
		return Ticket{}, false, err
	} else if t, ok := m[clientID]; ok {
		return t, false, nil
	}
	now := time.Now().UTC()
	res, err := s.db.Exec(`INSERT INTO tickets (client_id, note, created_at) VALUES (?,?,?)`, clientID, note, now.Unix())
	if err != nil {
		return Ticket{}, false, err
	}
	id, _ := res.LastInsertId()
	return Ticket{ID: id, ClientID: clientID, Note: note, CreatedAt: now}, true, nil
}
