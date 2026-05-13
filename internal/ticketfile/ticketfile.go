package ticketfile

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const dir = "tickets"

// Write crea el archivo de folio del ticket en el directorio tickets/
// el nombre del archivo sigue el formato USUARIO-INGENIERO-SUCURSAL-TICKET.txt
// este archivo sirve como comprobante fisico de que el ticket fue creado
func Write(idUsuario, idIngeniero, idSucursal int, idTicket int64) (string, error) {
	return writeAt(dir, idUsuario, idIngeniero, idSucursal, idTicket)
}

// writeAt es el helper interno que crea el archivo en el directorio dado
// separado de Write para facilitar las pruebas con directorios temporales
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
