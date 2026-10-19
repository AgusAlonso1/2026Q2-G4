// Callback HTTPS de Cognito (workaround del redirect).

const { COGNITO_DOMAIN_URL, CLIENT_ID, CLIENT_SECRET, CALLBACK_URL } = process.env;
const FRONTEND_URL = process.env.FRONTEND_URL.replace(/\/$/, "");

export const handler = async (event) => {
  if (event.rawPath === "/logout") {
    return redirect(`${FRONTEND_URL}/`);
  }

  const params = event.queryStringParameters ?? {};
  const state = params.state ?? "";

  if (params.error) {
    return back({ error: params.error, state });
  }
  if (!params.code) {
    return back({ error: "missing_code", state });
  }

  const res = await fetch(`${COGNITO_DOMAIN_URL}/oauth2/token`, {
    method: "POST",
    headers: {
      "Content-Type": "application/x-www-form-urlencoded",
      Authorization: `Basic ${Buffer.from(`${CLIENT_ID}:${CLIENT_SECRET}`).toString("base64")}`,
    },
    body: new URLSearchParams({
      grant_type: "authorization_code",
      client_id: CLIENT_ID,
      code: params.code,
      redirect_uri: CALLBACK_URL,
    }),
    signal: AbortSignal.timeout(5000),
  });

  if (!res.ok) {
    return back({ error: "token_exchange_failed", state });
  }

  const tokens = await res.json();
  return back({
    access_token: tokens.access_token,
    id_token: tokens.id_token,
    expires_in: tokens.expires_in,
    token_type: tokens.token_type,
    state,
  });
};

const back = (fragment) => redirect(`${FRONTEND_URL}/#${new URLSearchParams(fragment)}`);

const redirect = (location) => ({
  statusCode: 302,
  headers: { Location: location, "Cache-Control": "no-store" },
});
