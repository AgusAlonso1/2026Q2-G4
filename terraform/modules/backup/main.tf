# Backups de RDS con AWS Backup, copiados a otra región. Los backups automáticos de RDS (7 días)
# viven con la instancia y en su región: si se borra la base o se cae us-east-1, se pierden. Estas
# copias están en un vault aparte y en us-west-2, que es la otra región que habilita AWS Academy:
# es la estrategia de backup & restore (cold standby) de la clase de Disaster Recovery.

data "aws_region" "dr" {
  provider = aws.dr
}

# force_destroy: el workflow Destroy tiene que poder borrar todo, también las copias. En una
# cuenta propia no iría, y el vault de DR llevaría un vault lock (AWS Academy no lo deja crear).
resource "aws_backup_vault" "primary" {
  name          = var.name_prefix
  force_destroy = true
}

resource "aws_backup_vault" "dr" {
  provider = aws.dr

  name          = "${var.name_prefix}-dr"
  force_destroy = true
}

resource "aws_backup_plan" "this" {
  name = var.name_prefix

  rule {
    rule_name         = "diario"
    target_vault_name = aws_backup_vault.primary.name
    schedule          = var.schedule
    start_window      = 60
    completion_window = 180

    lifecycle {
      delete_after = var.retention_days
    }

    copy_action {
      destination_vault_arn = aws_backup_vault.dr.arn

      lifecycle {
        delete_after = var.retention_days
      }
    }
  }
}

resource "aws_backup_selection" "rds" {
  name         = "${var.name_prefix}-rds"
  plan_id      = aws_backup_plan.this.id
  iam_role_arn = var.role_arn
  resources    = [var.db_instance_arn]
}
