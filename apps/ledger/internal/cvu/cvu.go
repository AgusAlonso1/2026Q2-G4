package cvu

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"strconv"
)

const Largo = 22

const prefijo = "000"

var (
	pesosBloque1 = []int{7, 1, 3, 9, 7, 1, 3}
	pesosBloque2 = []int{3, 9, 7, 1, 3, 9, 7, 1, 3, 9, 7, 1, 3}
)

func digito(numeros string, pesos []int) byte {
	suma := 0
	for i, p := range pesos {
		suma += int(numeros[i]-'0') * p
	}
	return byte('0' + (10-suma%10)%10)
}

func BilleteraValida(nombre string) bool {
	if len(nombre) < 2 || len(nombre) > 15 || nombre[0] < 'a' || nombre[0] > 'z' {
		return false
	}
	for i := 0; i < len(nombre); i++ {
		if (nombre[i] < 'a' || nombre[i] > 'z') && (nombre[i] < '0' || nombre[i] > '9') {
			return false
		}
	}
	return true
}

func EntidadDeBilletera(nombre string) string {
	suma := sha256.Sum256([]byte(nombre))
	return fmt.Sprintf("%04d", binary.BigEndian.Uint32(suma[:4])%10000)
}

func EntidadValida(entidad string) bool {
	return len(entidad) == 4 && soloDigitos(entidad)
}

func Armar(entidad string, cuenta int64) string {
	bloque1 := prefijo + entidad
	bloque2 := fmt.Sprintf("%013d", cuenta)
	return bloque1 + string(digito(bloque1, pesosBloque1)) + bloque2 + string(digito(bloque2, pesosBloque2))
}

func Validar(clave string) error {
	if len(clave) != Largo || !soloDigitos(clave) {
		return fmt.Errorf("una CBU o un CVU tiene %d numeros", Largo)
	}
	if clave[7] != digito(clave[:7], pesosBloque1) || clave[21] != digito(clave[8:21], pesosBloque2) {
		return fmt.Errorf("los digitos verificadores no coinciden")
	}
	return nil
}

func Partes(clave string) (entidad string, cuenta int64, ok bool) {
	if Validar(clave) != nil || clave[:3] != prefijo {
		return "", 0, false
	}
	cuenta, err := strconv.ParseInt(clave[8:21], 10, 64)
	if err != nil {
		return "", 0, false
	}
	return clave[3:7], cuenta, true
}

func soloDigitos(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}
