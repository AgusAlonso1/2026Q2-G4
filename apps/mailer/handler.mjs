import { CognitoIdentityProviderClient, ListUsersCommand } from "@aws-sdk/client-cognito-identity-provider";
import { SESv2Client, SendEmailCommand } from "@aws-sdk/client-sesv2";

const { USER_POOL_ID, SENDER } = process.env;

const cognito = new CognitoIdentityProviderClient({});
const ses = new SESv2Client({});

export const pesos = (centavos) => {
  const signo = centavos < 0 ? "-" : "";
  const abs = Math.abs(centavos);
  const enteros = new Intl.NumberFormat("es-AR").format(Math.trunc(abs / 100));
  return `${signo}$ ${enteros},${String(abs % 100).padStart(2, "0")}`;
};

export const mensaje = (e) => {
  const monto = pesos(e.monto_centavos);
  const motivo = e.motivo ? ` Motivo: ${e.motivo}.` : "";
  const ref = e.referencia ? ` Referencia: ${e.referencia}.` : "";

  if (e.tipo === "transferencia" && e.estado === "liquidada") {
    return { para: e.titulares?.destino, asunto: `Recibiste ${monto}`, texto: `Recibiste una transferencia de ${monto} desde la cuenta ${e.origen_id}.${ref}` };
  }
  if (e.tipo === "ingreso" && e.estado === "atribuida") {
    return { para: e.titulares?.destino, asunto: `Ingresaron ${monto}`, texto: `Ingresaron ${monto} a tu cuenta desde tu banco.${ref}` };
  }
  if (e.tipo === "egreso" && e.estado === "acreditada") {
    return { para: e.titulares?.origen, asunto: `Tu retiro de ${monto} se acreditó`, texto: `Tu retiro de ${monto} se acreditó en la cuenta de destino.${ref}` };
  }
  if (e.tipo === "egreso" && (e.estado === "devuelta" || e.estado === "rechazada")) {
    return { para: e.titulares?.origen, asunto: `Tu retiro de ${monto} no se pudo completar`, texto: `Tu retiro de ${monto} no se pudo completar y el dinero sigue en tu cuenta.${motivo}${ref}` };
  }
  return null;
};

const emailDe = async (sub) => {
  const { Users } = await cognito.send(new ListUsersCommand({
    UserPoolId: USER_POOL_ID,
    Filter: `sub = "${sub.replace(/"/g, "")}"`,
    Limit: 1,
  }));
  return Users?.[0]?.Attributes?.find((a) => a.Name === "email")?.Value;
};

const procesar = async (evento) => {
  const m = mensaje(evento);
  if (!m?.para) {
    return;
  }
  const email = await emailDe(m.para);
  if (!email) {
    console.warn(JSON.stringify({ msg: "titular sin email en Cognito", evento: evento.evento_id }));
    return;
  }

  try {
    await ses.send(new SendEmailCommand({
      FromEmailAddress: SENDER,
      Destination: { ToAddresses: [email] },
      Content: {
        Simple: {
          Subject: { Data: m.asunto, Charset: "UTF-8" },
          Body: { Text: { Data: `${m.texto}\n\nOrden ${evento.orden_id}.`, Charset: "UTF-8" } },
        },
      },
    }));
  } catch (err) {
    if (err.name === "MessageRejected") {
      console.warn(JSON.stringify({ msg: "SES rechazo el mail", evento: evento.evento_id, error: err.message }));
      return;
    }
    throw err;
  }
  console.log(JSON.stringify({ msg: "mail enviado", evento: evento.evento_id }));
};

export const handler = async (event) => {
  if (!SENDER) {
    console.warn(JSON.stringify({ msg: "sin SENDER: no se mandan mails" }));
    return;
  }
  for (const record of event.Records) {
    await procesar(JSON.parse(record.Sns.Message));
  }
};
