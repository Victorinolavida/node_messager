package db

import (
	"encoding/json"
	"fmt"
	"os"
)

type SeedIngeniero struct {
	ID         int    `json:"id"`
	Nombre     string `json:"nombre"`
	SucursalID int    `json:"sucursal_id"`
}

type SeedUsuario struct {
	ID         int    `json:"id"`
	Nombre     string `json:"nombre"`
	SucursalID int    `json:"sucursal_id"`
}

type SeedDispositivo struct {
	ID          int    `json:"id"`
	Nombre      string `json:"nombre"`
	Tipo        string `json:"tipo"`
	SucursalID  int    `json:"sucursal_id"`
	IngenieroID int    `json:"ingeniero_id"`
}

type SeedData struct {
	Ingenieros   []SeedIngeniero   `json:"ingenieros"`
	Usuarios     []SeedUsuario     `json:"usuarios"`
	Dispositivos []SeedDispositivo `json:"dispositivos"`
}

func LoadSeedData(path string) (SeedData, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return SeedData{}, fmt.Errorf("read seed file: %w", err)
	}
	var data SeedData
	if err := json.Unmarshal(b, &data); err != nil {
		return SeedData{}, fmt.Errorf("parse seed file: %w", err)
	}
	return data, nil
}

// Seed inserta datos iniciales solo para el nodo indicado. Omite si el nodo ya tiene datos.
func (d *DB) Seed(nodeID int, data SeedData) error {
	if n, err := d.CountIngenieros(); err != nil || n > 0 {
		return err
	}
	for _, r := range data.Ingenieros {
		if r.SucursalID != nodeID {
			continue
		}
		if err := d.InsertIngeniero(r.ID, r.Nombre, r.SucursalID); err != nil {
			return fmt.Errorf("seed ingeniero %d: %w", r.ID, err)
		}
	}
	for _, r := range data.Usuarios {
		if r.SucursalID != nodeID {
			continue
		}
		if err := d.InsertUsuario(r.ID, r.Nombre, r.SucursalID); err != nil {
			return fmt.Errorf("seed usuario %d: %w", r.ID, err)
		}
	}
	for _, r := range data.Dispositivos {
		if r.SucursalID != nodeID {
			continue
		}
		if err := d.InsertDispositivo(r.ID, r.Nombre, r.Tipo, r.SucursalID, r.IngenieroID); err != nil {
			return fmt.Errorf("seed dispositivo %d: %w", r.ID, err)
		}
	}
	return nil
}
