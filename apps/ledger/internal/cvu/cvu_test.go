package cvu

import "testing"

func TestArmarDaUnCVUValidoQueSeDesarma(t *testing.T) {
	c := Armar("4233", 17)
	if len(c) != Largo {
		t.Fatalf("largo %d: %s", len(c), c)
	}
	if err := Validar(c); err != nil {
		t.Fatalf("Validar(%s): %v", c, err)
	}
	entidad, cuenta, ok := Partes(c)
	if !ok || entidad != "4233" || cuenta != 17 {
		t.Fatalf("Partes(%s) = %s, %d, %v", c, entidad, cuenta, ok)
	}
}

func TestValidarAceptaUnaCBUBancaria(t *testing.T) {
	if err := Validar("2850590940090418135201"); err != nil {
		t.Fatalf("CBU valida rechazada: %v", err)
	}
	if _, _, ok := Partes("2850590940090418135201"); ok {
		t.Fatal("una CBU bancaria no es un CVU")
	}
}

func TestValidarRechazaUnErrorDeTipeo(t *testing.T) {
	c := []byte(Armar("4233", 17))
	c[15] = '9'
	if err := Validar(string(c)); err == nil {
		t.Fatalf("se esperaba error para %s", c)
	}
	for _, mal := range []string{"", "123", "00000000000000000000000", "0000000000000000000000a"} {
		if err := Validar(mal); err == nil {
			t.Errorf("Validar(%q) no dio error", mal)
		}
	}
}

func TestLaEntidadSaleDelNombreDeLaBilletera(t *testing.T) {
	norte, sur := EntidadDeBilletera("norte"), EntidadDeBilletera("sur")
	if !EntidadValida(norte) || !EntidadValida(sur) {
		t.Fatalf("entidades invalidas: %q, %q", norte, sur)
	}
	if norte != EntidadDeBilletera("norte") {
		t.Fatal("la misma billetera dio dos entidades")
	}
	if norte == sur {
		t.Fatalf("norte y sur dieron la misma entidad %s", norte)
	}
}
