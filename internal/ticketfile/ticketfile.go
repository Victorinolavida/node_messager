package ticketfile

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const dir = "tickets"

// Write creates the folio file under the default `tickets/` directory.
func Write(idUsuario, idIngeniero, idSucursal int, idTicket int64) (string, error) {
	return writeAt(dir, idUsuario, idIngeniero, idSucursal, idTicket)
}

func writeAt(baseDir string, idUsuario, idIngeniero, idSucursal int, idTicket int64) (string, error) {
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return "", err
	}
	filename := fmt.Sprintf("%d-%d-%d-%d.txt", idUsuario, idIngeniero, idSucursal, idTicket)
	path := filepath.Join(baseDir, filename)
	content := fmt.Sprintf(
		"FOLIO: %s\nUsuario: %d\nIngeniero: %d\nSucursal: %d\nTicket: %d\nFecha: %s\n",
		filename, idUsuario, idIngeniero, idSucursal, idTicket,
		time.Now().UTC().Format(time.RFC3339),
	)
	return filename, os.WriteFile(path, []byte(content), 0644)
}
