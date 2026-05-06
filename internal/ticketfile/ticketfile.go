package ticketfile

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const dir = "tickets"

// Write creates the folio file and returns the filename.
// Folio format: IDUSUARIO-IDINGENIERO-IDSUCURSAL-IDTICKET
func Write(idUsuario, idIngeniero, idSucursal int, idTicket int64) (string, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	filename := fmt.Sprintf("%d-%d-%d-%d.txt", idUsuario, idIngeniero, idSucursal, idTicket)
	path := filepath.Join(dir, filename)
	content := fmt.Sprintf(
		"FOLIO: %s\nUsuario: %d\nIngeniero: %d\nSucursal: %d\nTicket: %d\nFecha: %s\n",
		filename, idUsuario, idIngeniero, idSucursal, idTicket,
		time.Now().UTC().Format(time.RFC3339),
	)
	return filename, os.WriteFile(path, []byte(content), 0644)
}
