TF_DIR := terraform/organization
TF     ?= terraform
REGION ?= us-east-1

export REGION

.PHONY: help fmt fmt-check init validate plan apply destroy clean

help:
	@echo "make fmt        Formatea todo terraform/"
	@echo "make fmt-check  Verifica el formato sin modificar (lo mismo que corre el pipeline)"
	@echo "make init       Crea el bucket del state si no existe e inicializa terraform/organization"
	@echo "make validate   init sin backend + validate (no necesita AWS)"
	@echo "make plan       Genera el plan en terraform/organization/tfplan"
	@echo "make apply      Aplica el plan guardado"
	@echo "make destroy    Destruye toda la infraestructura y después el bucket del state"
	@echo "make clean      Borra .terraform/ y tfplan"

fmt:
	$(TF) fmt -recursive terraform

fmt-check:
	$(TF) fmt -check -recursive terraform

init:
	@bucket=$$(scripts/state-bucket.sh create) && \
	$(TF) -chdir=$(TF_DIR) init -backend-config="bucket=$$bucket"

validate:
	$(TF) -chdir=$(TF_DIR) init -backend=false
	$(TF) -chdir=$(TF_DIR) validate

plan: init
	$(TF) -chdir=$(TF_DIR) plan -out=tfplan

apply:
	$(TF) -chdir=$(TF_DIR) apply tfplan

destroy: init
	$(TF) -chdir=$(TF_DIR) destroy $(DESTROY_FLAGS)
	@if [ -n "$$($(TF) -chdir=$(TF_DIR) state list)" ]; then \
	  echo "Quedan recursos en el state: no se borra el bucket" >&2; exit 1; \
	fi
	scripts/state-bucket.sh delete

clean:
	rm -rf $(TF_DIR)/.terraform $(TF_DIR)/tfplan
