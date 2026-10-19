TF_DIR := terraform/organization
TF     ?= terraform

.PHONY: help fmt fmt-check init validate plan apply destroy clean

help:
	@echo "make fmt        Formatea todo terraform/"
	@echo "make fmt-check  Verifica el formato sin modificar (lo mismo que corre el pipeline)"
	@echo "make init       Inicializa terraform/organization"
	@echo "make validate   init + validate"
	@echo "make plan       Genera el plan en terraform/organization/tfplan"
	@echo "make apply      Aplica el plan guardado"
	@echo "make destroy    Destruye toda la infraestructura"
	@echo "make clean      Borra .terraform/ y tfplan"

fmt:
	$(TF) fmt -recursive terraform

fmt-check:
	$(TF) fmt -check -recursive terraform

init:
	$(TF) -chdir=$(TF_DIR) init

validate: init
	$(TF) -chdir=$(TF_DIR) validate

plan: init
	$(TF) -chdir=$(TF_DIR) plan -out=tfplan

apply:
	$(TF) -chdir=$(TF_DIR) apply tfplan

destroy:
	$(TF) -chdir=$(TF_DIR) destroy

clean:
	rm -rf $(TF_DIR)/.terraform $(TF_DIR)/tfplan
