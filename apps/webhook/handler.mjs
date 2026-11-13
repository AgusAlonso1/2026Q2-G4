import { createHmac } from "node:crypto"
import { GetSecretValueCommand, SecretsManagerClient } from "@aws-sdk/client-secrets-manager";

const { WEBHOOK_URL, WEBHOOK_SECRET_ARN } = process.env;
const CABECERA_FIRMA = "X-Ledger-Signature";

const secretsManager = new SecretsManagerClient({});
let secreto;

const leerSecreto = async () => {
  secreto ??= (await secretsManager.send(new GetSecretValueCommand({ SecretId: WEBHOOK_SECRET_ARN }))).SecretString;
  return secreto;
};

export const firmar = (cuerpo, clave) => createHmac("sha256", clave).update(cuerpo).digest("hex");

const entregar = async (record, clave) => {
  const cuerpo = record.body;
  const res = await fetch(WEBHOOK_URL, {
    method: "POST",
    headers: { "Content-Type": "application/json", [CABECERA_FIRMA]: firmar(cuerpo, clave) },
    body: cuerpo,
    signal: AbortSignal.timeout(5000),
  });
  if (!res.ok) {
    throw new Error(`el webhook respondio ${res.status}`);
  }
};

export const handler = async (event) => {
  if (!WEBHOOK_URL) {
    console.log(JSON.stringify({ msg: "sin WEBHOOK_URL: se descartan los avisos", cantidad: event.Records.length }));
    return { batchItemFailures: [] };
  }

  const clave = await leerSecreto();

  const resultados = await Promise.allSettled(event.Records.map((r) => entregar(r, clave)));

  const fallidos = [];
  resultados.forEach((resultado, i) => {
    if (resultado.status === "rejected") {
      const record = event.Records[i];
      console.warn(JSON.stringify({
        msg: "webhook no entregado",
        mensaje: record.messageId,
        intento: Number(record.attributes.ApproximateReceiveCount),
        error: String(resultado.reason?.message ?? resultado.reason),
      }));
      fallidos.push({ itemIdentifier: record.messageId });
    }
  });
  return { batchItemFailures: fallidos };
};
