import {
  estado, fijarBase, fijarTokens, fijarCuenta, fijarClaves, cerrarSesion, hayToken, haySesion,
  emailDeLaSesion, pedir, consultarSalud, vigente, alCaerSesion, alPerderCuenta, mensajeDeError,
} from './api.js';
import { leerConfig, irAlLogin, irAlLogout, leerVuelta } from './cognito.js';
import { el, vaciar, avisar, iniciales } from './formato.js';
import * as usuario from './usuario.js';
import { panel } from './panel.js';

const acceso = document.getElementById('acceso');
const aplicacion = document.getElementById('aplicacion');
const hoja = document.getElementById('hoja');
const nav = document.getElementById('nav');
const navMovil = document.getElementById('nav-movil');

const SECCIONES = [
  { hash: '#/inicio', texto: 'Inicio', icono: '🏠', pintar: usuario.inicio },
  { hash: '#/enviar', texto: 'Enviar', icono: '↗', pintar: usuario.enviar },
  { hash: '#/ingresar', texto: 'Ingresar', icono: '↓', pintar: usuario.ingresar },
  { hash: '#/retirar', texto: 'Retirar', icono: '↑', pintar: usuario.retirar },
  { hash: '#/actividad', texto: 'Actividad', icono: '☰', pintar: usuario.actividad },
  { hash: '#/panel', texto: 'Operación', icono: '⚙', pintar: panel },
];

let config = null;

const formAcceso = document.getElementById('form-acceso');
const errorAcceso = document.getElementById('acceso-error');
const formAlta = document.getElementById('form-alta');
const errorAlta = document.getElementById('alta-error');
const campoAlias = document.getElementById('alta-alias');

function mostrar(nodo, texto) {
  nodo.textContent = texto;
  nodo.hidden = !texto;
}

function mostrarAcceso(error = '') {
  aplicacion.hidden = true;
  acceso.hidden = false;
  formAlta.hidden = true;
  formAcceso.hidden = false;
  mostrar(errorAcceso, error);
  document.getElementById('acceso-entrar').disabled = !config;
}

function mostrarAlta() {
  aplicacion.hidden = true;
  acceso.hidden = false;
  formAcceso.hidden = true;
  formAlta.hidden = false;
  mostrar(errorAlta, '');

  const email = emailDeLaSesion();
  document.getElementById('alta-quien').textContent = email
    ? `Entraste como ${email}. Elegí el alias con el que te van a ver los demás.`
    : 'Elegí el alias con el que te van a ver los demás.';
  if (!campoAlias.value) campoAlias.value = email.split('@')[0].toLowerCase().replace(/[^a-z0-9._-]/g, '');
  campoAlias.focus();
}

formAcceso.addEventListener('submit', (ev) => {
  ev.preventDefault();
  if (config) irAlLogin(config);
});

formAlta.addEventListener('submit', async (ev) => {
  ev.preventDefault();
  mostrar(errorAlta, '');
  const boton = formAlta.querySelector('button[type=submit]');
  boton.disabled = true;
  try {
    const r = await pedir('POST', '/cuentas', {
      cuerpo: { alias: campoAlias.value.trim(), tipo: 'usuario', saldo_inicial_centavos: 5000000 },
    });
    if (r.codigo === 401 || r.codigo === 0) return;
    if (!r.ok && r.datos?.codigo !== 'ya_tiene_cuenta') throw new Error(mensajeDeError(r));
    usuario.olvidarAgenda();
    if (r.ok) avisar(`Cuenta ${r.datos.alias} creada`);
    await arrancar();
  } catch (err) {
    mostrar(errorAlta, err.message);
  } finally {
    boton.disabled = false;
  }
});

document.getElementById('alta-salir').addEventListener('click', salir);

const dialogoServidor = document.getElementById('dialogo-servidor');
const estadoServidor = document.getElementById('servidor-estado');

async function abrirServidor() {
  document.getElementById('servidor-base').value = estado.base || '(sin config.json)';
  estadoServidor.textContent = 'Probando…';
  dialogoServidor.showModal();
  if (!estado.base) {
    estadoServidor.textContent = 'No hay dirección de la API: falta config.json';
    return;
  }
  try {
    const datos = await consultarSalud();
    estadoServidor.textContent = datos.estado === 'ok'
      ? `Responde: versión ${datos.version}, base ${datos.base}`
      : `Responde, pero está ${datos.estado}: base ${datos.base}`;
  } catch (err) {
    estadoServidor.textContent = err.message;
  }
}

document.getElementById('acceso-servidor').addEventListener('click', abrirServidor);
document.getElementById('alta-servidor').addEventListener('click', abrirServidor);

const dialogoClaves = document.getElementById('dialogo-claves');
dialogoClaves.addEventListener('close', () => {
  if (dialogoClaves.returnValue === 'guardar') {
    fijarClaves({
      admin: document.getElementById('clave-admin').value,
      mock: document.getElementById('clave-mock').value,
    });
    avisar('Claves guardadas');
    pintar();
  }
});

function abrirClaves() {
  document.getElementById('clave-admin').value = estado.admin;
  document.getElementById('clave-mock').value = estado.mock;
  dialogoClaves.returnValue = '';
  dialogoClaves.showModal();
}

// Las tarjetas del panel piden este diálogo por evento: importarlo desde panel.js haría una
// dependencia circular.
document.addEventListener('nucleo:abrir-claves', abrirClaves);

document.getElementById('menu-perfil').addEventListener('click', () => {
  const acciones = [
    ['Servidor', abrirServidor],
    ['Claves de operación', abrirClaves],
    ['Salir', salir],
  ];
  const menu = el('dialog', { clase: 'dialogo-menu' },
    el('div', { clase: 'dialogo' },
      el('h2', { texto: estado.cuenta?.alias || 'Tu cuenta' }),
      el('div', { clase: 'fila-botones' },
        ...acciones.map(([texto, fn]) => el('button', {
          clase: 'boton', texto,
          onclick: () => { menu.close(); menu.remove(); fn(); },
        })))));
  document.body.append(menu);
  menu.addEventListener('close', () => menu.remove());
  menu.showModal();
});

function salir() {
  cerrarSesion();
  usuario.olvidarAgenda();
  if (config) irAlLogout(config);
  else mostrarAcceso();
}

alCaerSesion((codigo) => {
  usuario.olvidarAgenda();
  mostrarAcceso(codigo === 'sesion_vencida' ? 'Tu sesión venció: volvé a ingresar' : 'Tu sesión no es válida: volvé a ingresar');
});

alPerderCuenta(() => {
  usuario.olvidarAgenda();
  mostrarAlta();
});

function construirNav() {
  vaciar(nav);
  vaciar(navMovil);
  for (const s of SECCIONES) {
    nav.append(el('button', { texto: s.texto, dataset: { hash: s.hash }, onclick: () => ir(s.hash) }));
    navMovil.append(el('button', { dataset: { hash: s.hash }, onclick: () => ir(s.hash) },
      el('span', { clase: 'icono', texto: s.icono }), s.texto));
  }
}

function ir(hash) {
  if (location.hash === hash) pintar();
  else location.hash = hash;
}

function marcarActiva(hash) {
  for (const b of document.querySelectorAll('[data-hash]')) {
    if (b.dataset.hash === hash) b.setAttribute('aria-current', 'page');
    else b.removeAttribute('aria-current');
  }
}

// Cada pantalla se dibuja en su propio contenedor: si una anterior termina tarde, escribe en uno
// que ya no está en la página.
async function pintar() {
  if (!vigente() || !haySesion()) return;

  const hash = location.hash || '#/inicio';
  const partes = hash.slice(2).split('/');
  const lienzo = el('div', { style: 'display:contents' });
  hoja.replaceChildren(lienzo);
  marcarActiva(`#/${partes[0]}`);

  try {
    if (partes[0] === 'operacion') {
      await usuario.operacion(lienzo, ir, partes[1], partes[2]);
      return;
    }
    const seccion = SECCIONES.find((s) => s.hash === `#/${partes[0]}`) || SECCIONES[0];
    await seccion.pintar(lienzo, ir);
  } catch (err) {
    if (!haySesion() || !lienzo.isConnected) return;
    lienzo.append(el('section', { clase: 'tarjeta' },
      el('p', { clase: 'error', texto: err.message || String(err) }),
      el('div', { clase: 'fila-botones', style: 'margin-top:.75rem' },
        el('button', { clase: 'boton', texto: 'Reintentar', onclick: pintar }),
        el('button', { clase: 'boton', texto: 'Servidor', onclick: abrirServidor }))));
  }
}

window.addEventListener('hashchange', pintar);

let vencimiento = 0;

function entrar() {
  clearTimeout(vencimiento);
  vencimiento = setTimeout(vigente, Math.max(0, estado.expira - Date.now()) + 1000);
  acceso.hidden = true;
  aplicacion.hidden = false;
  document.getElementById('menu-perfil').textContent = iniciales(estado.cuenta?.alias);
  construirNav();
  if (!location.hash.startsWith('#/') || location.hash === '#/') location.hash = '#/inicio';
  else pintar();
}

// El token guardado puede haber vencido o la cuenta haber sido borrada mientras la pestaña
// estaba cerrada: se confirma contra la API antes de mostrar nada.
async function arrancar() {
  if (!hayToken()) {
    mostrarAcceso();
    return;
  }
  let r;
  try {
    r = await pedir('GET', '/sesiones/actual');
  } catch (err) {
    mostrarAcceso(err.message);
    return;
  }
  if (r.codigo === 401 || r.codigo === 0) return;
  if (!r.ok) {
    mostrarAcceso(mensajeDeError(r));
    return;
  }
  if (!r.datos.sesion) {
    mostrarAcceso('La API corre sin sesiones (AUTH_ABIERTA=1): esta app necesita Cognito');
    return;
  }
  if (r.datos.cuenta_id == null) {
    mostrarAlta();
    return;
  }
  fijarCuenta({ id: r.datos.cuenta_id, alias: r.datos.alias });
  entrar();
}

async function iniciar() {
  const vuelta = leerVuelta();
  try {
    config = await leerConfig();
    fijarBase(config.api_url);
  } catch (err) {
    mostrarAcceso(err.message);
    return;
  }
  if (vuelta?.error) {
    mostrarAcceso(vuelta.error);
    return;
  }
  if (vuelta?.tokens) fijarTokens(vuelta.tokens);
  await arrancar();
}

iniciar();
