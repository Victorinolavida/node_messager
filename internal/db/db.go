package db

import (
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

type DB struct {
	sql *sql.DB
}

func Open(name string) (*DB, error) {
	return openAt(fmt.Sprintf("data/%s.db", name))
}

func openAt(path string) (*DB, error) {
	sqlDB, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	sqlDB.SetMaxOpenConns(1)
	d := &DB{sql: sqlDB}
	if err := d.migrate(); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return d, nil
}

func (d *DB) Close() error { return d.sql.Close() }

// Version returns the current applied schema version.
func (d *DB) Version() (int, error) {
	var v int
	err := d.sql.QueryRow(`SELECT version FROM _schema_version LIMIT 1`).Scan(&v)
	if err != nil {
		return 0, nil // table not yet created
	}
	return v, nil
}

// migrations is ordered — append only, never modify existing entries.
var migrations = []struct {
	version int
	sql     string
}{
	{1, `
		CREATE TABLE IF NOT EXISTS INGENIEROS (
			id          INTEGER PRIMARY KEY,
			nombre      TEXT    NOT NULL,
			sucursal_id INTEGER NOT NULL,
			disponible  INTEGER NOT NULL DEFAULT 1
		);
		CREATE TABLE IF NOT EXISTS USUARIOS (
			id          INTEGER PRIMARY KEY,
			nombre      TEXT    NOT NULL,
			sucursal_id INTEGER NOT NULL
		);
		CREATE TABLE IF NOT EXISTS DISPOSITIVOS (
			id          INTEGER PRIMARY KEY,
			nombre      TEXT    NOT NULL,
			tipo        TEXT    NOT NULL,
			sucursal_id INTEGER NOT NULL
		);
		CREATE TABLE IF NOT EXISTS TICKETS (
			id             INTEGER PRIMARY KEY,
			id_usuario     INTEGER NOT NULL,
			id_ingeniero   INTEGER,
			id_sucursal    INTEGER NOT NULL,
			id_dispositivo INTEGER NOT NULL,
			estado         TEXT    NOT NULL DEFAULT 'ABIERTO',
			folio          TEXT,
			created_at     TEXT    NOT NULL,
			closed_at      TEXT
		);
	`},
	{2, `ALTER TABLE DISPOSITIVOS ADD COLUMN ingeniero_id INTEGER DEFAULT 0;`},
}

func (d *DB) migrate() error {
	// create version tracking table
	if _, err := d.sql.Exec(`
		CREATE TABLE IF NOT EXISTS _schema_version (version INTEGER NOT NULL)
	`); err != nil {
		return fmt.Errorf("create version table: %w", err)
	}

	current, err := d.Version()
	if err != nil {
		return fmt.Errorf("read schema version: %w", err)
	}

	for _, m := range migrations {
		if m.version <= current {
			continue
		}
		if _, err := d.sql.Exec(m.sql); err != nil {
			return fmt.Errorf("migration v%d: %w", m.version, err)
		}
		if current == 0 {
			_, err = d.sql.Exec(`INSERT INTO _schema_version(version) VALUES(?)`, m.version)
		} else {
			_, err = d.sql.Exec(`UPDATE _schema_version SET version=?`, m.version)
		}
		if err != nil {
			return fmt.Errorf("update schema version to %d: %w", m.version, err)
		}
		current = m.version
	}
	return nil
}

// ── INGENIEROS ────────────────────────────────────────────────────────────────

func (d *DB) InsertIngeniero(id int, nombre string, sucursalID int) error {
	_, err := d.sql.Exec(
		`INSERT OR IGNORE INTO INGENIEROS(id,nombre,sucursal_id,disponible) VALUES(?,?,?,1)`,
		id, nombre, sucursalID,
	)
	return err
}

func (d *DB) GetIngenieros() ([]Ingeniero, error) {
	rows, err := d.sql.Query(`SELECT id,nombre,sucursal_id,disponible FROM INGENIEROS`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Ingeniero
	for rows.Next() {
		var r Ingeniero
		if err := rows.Scan(&r.ID, &r.Nombre, &r.SucursalID, &r.Disponible); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (d *DB) GetIngenieroDisponible() (*Ingeniero, error) {
	row := d.sql.QueryRow(`SELECT id,nombre,sucursal_id,disponible FROM INGENIEROS WHERE disponible=1 LIMIT 1`)
	var r Ingeniero
	if err := row.Scan(&r.ID, &r.Nombre, &r.SucursalID, &r.Disponible); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &r, nil
}

func (d *DB) SetIngenieroDisponible(id, disponible int) error {
	_, err := d.sql.Exec(`UPDATE INGENIEROS SET disponible=? WHERE id=?`, disponible, id)
	return err
}

func (d *DB) CountIngenieros() (int, error) {
	row := d.sql.QueryRow(`SELECT COUNT(*) FROM INGENIEROS`)
	var n int
	return n, row.Scan(&n)
}

// ── USUARIOS ──────────────────────────────────────────────────────────────────

func (d *DB) InsertUsuario(id int, nombre string, sucursalID int) error {
	_, err := d.sql.Exec(
		`INSERT OR IGNORE INTO USUARIOS(id,nombre,sucursal_id) VALUES(?,?,?)`,
		id, nombre, sucursalID,
	)
	return err
}

func (d *DB) GetUsuarios() ([]Usuario, error) {
	rows, err := d.sql.Query(`SELECT id,nombre,sucursal_id FROM USUARIOS`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Usuario
	for rows.Next() {
		var r Usuario
		if err := rows.Scan(&r.ID, &r.Nombre, &r.SucursalID); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ── DISPOSITIVOS ──────────────────────────────────────────────────────────────

func (d *DB) InsertDispositivo(id int, nombre, tipo string, sucursalID, ingenieroID int) error {
	_, err := d.sql.Exec(
		`INSERT OR IGNORE INTO DISPOSITIVOS(id,nombre,tipo,sucursal_id,ingeniero_id) VALUES(?,?,?,?,?)`,
		id, nombre, tipo, sucursalID, ingenieroID,
	)
	return err
}

func (d *DB) GetDispositivos() ([]Dispositivo, error) {
	rows, err := d.sql.Query(`SELECT id,nombre,tipo,sucursal_id,COALESCE(ingeniero_id,0) FROM DISPOSITIVOS`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Dispositivo
	for rows.Next() {
		var r Dispositivo
		if err := rows.Scan(&r.ID, &r.Nombre, &r.Tipo, &r.SucursalID, &r.IngenieroID); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (d *DB) CountDispositivosByIngeniero(ingenieroID int) (int, error) {
	row := d.sql.QueryRow(`SELECT COUNT(*) FROM DISPOSITIVOS WHERE ingeniero_id=?`, ingenieroID)
	var n int
	return n, row.Scan(&n)
}

func (d *DB) CountDispositivos() (int, error) {
	row := d.sql.QueryRow(`SELECT COUNT(*) FROM DISPOSITIVOS`)
	var n int
	return n, row.Scan(&n)
}

// ── TICKETS ───────────────────────────────────────────────────────────────────

func (d *DB) InsertTicket(idUsuario, idIngeniero, idSucursal, idDispositivo int) (int64, error) {
	res, err := d.sql.Exec(
		`INSERT INTO TICKETS(id_usuario,id_ingeniero,id_sucursal,id_dispositivo,estado,created_at)
		 VALUES(?,?,?,?,'ABIERTO',?)`,
		idUsuario, idIngeniero, idSucursal, idDispositivo,
		time.Now().UTC().Format(time.RFC3339),
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (d *DB) InsertTicketFull(t Ticket) error {
	_, err := d.sql.Exec(
		`INSERT OR IGNORE INTO TICKETS(id,id_usuario,id_ingeniero,id_sucursal,id_dispositivo,estado,folio,created_at,closed_at)
		 VALUES(?,?,?,?,?,?,?,?,?)`,
		t.ID, t.IDUsuario, t.IDIngeniero, t.IDSucursal, t.IDDispositivo,
		t.Estado, t.Folio, t.CreatedAt, t.ClosedAt,
	)
	return err
}

func (d *DB) UpdateTicketFolio(id int64, folio string) error {
	_, err := d.sql.Exec(`UPDATE TICKETS SET folio=? WHERE id=?`, folio, id)
	return err
}

func (d *DB) CloseTicket(id int64) error {
	_, err := d.sql.Exec(
		`UPDATE TICKETS SET estado='CERRADO', closed_at=? WHERE id=?`,
		time.Now().UTC().Format(time.RFC3339), id,
	)
	return err
}

func (d *DB) GetTickets() ([]Ticket, error) {
	rows, err := d.sql.Query(
		`SELECT id,id_usuario,id_ingeniero,id_sucursal,id_dispositivo,estado,COALESCE(folio,''),created_at,COALESCE(closed_at,'')
		 FROM TICKETS`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTickets(rows)
}

func (d *DB) GetOpenTicketsBySucursal(sucursalID int) ([]Ticket, error) {
	rows, err := d.sql.Query(
		`SELECT id,id_usuario,id_ingeniero,id_sucursal,id_dispositivo,estado,COALESCE(folio,''),created_at,COALESCE(closed_at,'')
		 FROM TICKETS WHERE estado='ABIERTO' AND id_sucursal=?`,
		sucursalID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTickets(rows)
}

func scanTickets(rows *sql.Rows) ([]Ticket, error) {
	var out []Ticket
	for rows.Next() {
		var t Ticket
		if err := rows.Scan(&t.ID, &t.IDUsuario, &t.IDIngeniero, &t.IDSucursal,
			&t.IDDispositivo, &t.Estado, &t.Folio, &t.CreatedAt, &t.ClosedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// ── Domain types ──────────────────────────────────────────────────────────────

type Ingeniero struct {
	ID         int
	Nombre     string
	SucursalID int
	Disponible int
}

type Usuario struct {
	ID         int
	Nombre     string
	SucursalID int
}

type Dispositivo struct {
	ID          int
	Nombre      string
	Tipo        string
	SucursalID  int
	IngenieroID int
}

type Ticket struct {
	ID            int64
	IDUsuario     int
	IDIngeniero   int
	IDSucursal    int
	IDDispositivo int
	Estado        string
	Folio         string
	CreatedAt     string
	ClosedAt      string
}
