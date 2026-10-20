import { pedir, exigir, estado, claveDeIntento, mensajeDeError } from './api.js';
import {
  el, enPesos, conSigno, aCentavos, cuando, fechaCompleta, desenlace, periodoActual,
  nombreDelPeriodo,
} from './formato.js';

const cuentaID = () => estado.cuenta?.id;

// Los movimientos traen contraparte_id, no un alias: el padrón traduce eso a "Para juan.perez".
// Se cachea y se vuelve a pedir sólo si aparece una cuenta que no conoce.
let padron = null;

async function agenda(requeridas = []) {
  if (!padron || requeridas.some((id) => !padron.has(id))) {
    const { cuentas } = await exigir('GET', '/cuentas?limite=200');
    padron = new Map(cuentas.map((c) => [c.id, c]));
  }
  return padron;
}

const contrapartes = (movimientos) => (movimientos || []).map((m) => m.contraparte_id);

export function olvidarAgenda() { padron = null; }

const TIPO_POR_TRAMO = {
  transferencia: 'transferencia',
  semilla: 'transferencia',
  reserva: 'egreso',
  reversa: 'egreso',
  liquidacion: 'egreso',
  ingreso_banco: 'ingreso',
  ingreso_acreditacion: 'ingreso',
};

export async function inicio(hoja, ir) {
  const saldo = el('section', { clase: 'tarjeta saldo' },
    el('div', { clase: 'rotulo', texto: 'Dinero disponible' }),
    el('div', { clase: 'importe', texto: '—' }),
    el('div', { clase: 'pie', texto: estado.cuenta?.alias || '' }));

  const acciones = el('section', { clase: 'acciones' },
    boton('↗', 'Enviar', () => ir('#/enviar')),
    boton('↓', 'Ingresar', () => ir('#/ingresar')),
    boton('↑', 'Retirar', () => ir('#/retirar')),
    boton('☰', 'Actividad', () => ir('#/actividad')));

  const actividad = el('section', { clase: 'tarjeta' },
    el('div', { clase: 'cabecera-tarjeta' },
      el('h2', { texto: 'Últimos movimientos' }),
      el('button', { clase: 'enlace', texto: 'Ver todo', onclick: () => ir('#/actividad') })),
    el('div', { clase: 'lista' }, el('p', { clase: 'vacio', texto: 'Cargando…' })));

  hoja.append(saldo, acciones, actividad);

  const [s, movs] = await Promise.all([
    exigir('GET', `/cuentas/${cuentaID()}/saldo`),
    exigir('GET', `/cuentas/${cuentaID()}/movimientos?limite=6`),
  ]);
  const contactos = await agenda(contrapartes(movs.movimientos));

  saldo.querySelector('.importe').textContent = enPesos(s.saldo_vigente_centavos);
  pintarMovimientos(actividad.querySelector('.lista'), movs.movimientos, contactos, ir);
}

function boton(icono, texto, onclick) {
  return el('button', { clase: 'accion', onclick },
    el('span', { clase: 'icono', texto: icono }), texto);
}

export async function actividad(hoja, ir) {
  const caja = el('section', { clase: 'tarjeta' },
    el('div', { clase: 'cabecera-tarjeta' }, el('h1', { texto: 'Actividad' })),
    el('div', { clase: 'lista' }, el('p', { clase: 'vacio', texto: 'Cargando…' })));
  const mensual = el('section', { clase: 'tarjeta' });
  hoja.append(caja, mensual);

  const movs = await exigir('GET', `/cuentas/${cuentaID()}/movimientos?limite=100`);
  const contactos = await agenda(contrapartes(movs.movimientos));
  pintarMovimientos(caja.querySelector('.lista'), movs.movimientos, contactos, ir);
  await resumenMensual(mensual, `/cuentas/${cuentaID()}/movimientos?periodo=${periodoActual()}`, contactos, ir);
}

async function resumenMensual(caja, ruta, contactos, ir) {
  const lista = el('div', { clase: 'lista' }, el('p', { clase: 'vacio', texto: 'Cargando…' }));
  const titulo = el('h2', { texto: 'Resumen mensual' });
  const anterior = el('button', { clase: 'boton boton-chico', texto: '‹', title: 'Mes anterior', disabled: true });
  const siguiente = el('button', { clase: 'boton boton-chico', texto: '›', title: 'Mes siguiente', disabled: true });
  caja.replaceChildren(
    el('div', { clase: 'cabecera-tarjeta' }, titulo, el('div', { clase: 'fila-botones' }, anterior, siguiente)),
    lista);

  const e = await exigir('GET', ruta);
  contactos = await agenda(contrapartes(e.movimientos));
  titulo.textContent = `Resumen de ${nombreDelPeriodo(e.periodo).toLowerCase()}`;
  const navegar = (destino) => () => resumenMensual(caja, destino, contactos, ir).catch((err) => {
    lista.replaceChildren(el('p', { clase: 'error', texto: err.message }));
  });
  if (e._links?.periodo_anterior) {
    anterior.disabled = false;
    anterior.addEventListener('click', navegar(e._links.periodo_anterior));
  }
  if (e._links?.periodo_siguiente && e.periodo < periodoActual()) {
    siguiente.disabled = false;
    siguiente.addEventListener('click', navegar(e._links.periodo_siguiente));
  }
  pintarMovimientos(lista, e.movimientos, contactos, ir, 'Ese mes no tuvo movimientos');
}

function pintarMovimientos(lista, movimientos, contactos, ir, vacio = 'Todavía no hay movimientos') {
  lista.replaceChildren();
  if (!movimientos?.length) {
    lista.append(el('p', { clase: 'vacio', texto: vacio }));
    return;
  }
  for (const m of movimientos) {
    const entra = m.monto_centavos > 0;
    const tipo = TIPO_POR_TRAMO[m.tramo] || 'transferencia';
    lista.append(el('button', {
      clase: 'fila',
      onclick: () => ir(`#/operacion/${tipo}/${m.orden_id}`),
    },
      el('span', { clase: 'punto', texto: icono(tipo, entra) }),
      el('span', {},
        el('div', { clase: 'titulo', texto: tituloMovimiento(m, tipo, entra, contactos) }),
        el('div', { clase: 'sub', texto: cuando(m.creado_en) })),
      el('span', { clase: `monto ${entra ? 'monto-entra' : 'monto-sale'}`, texto: conSigno(m.monto_centavos) })));
  }
}

function icono(tipo, entra) {
  if (tipo === 'egreso') return '🏦';
  if (tipo === 'ingreso') return '💰';
  return entra ? '↓' : '↑';
}

function tituloMovimiento(m, tipo, entra, contactos) {
  if (tipo === 'egreso') return entra ? 'Retiro devuelto' : 'Retiro a cuenta bancaria';
  if (tipo === 'ingreso') return 'Ingreso de dinero';

  const otra = contactos?.get(m.contraparte_id);
  if (!otra || otra.tipo === 'interna') return entra ? 'Dinero acreditado' : 'Dinero debitado';
  return `${entra ? 'De' : 'Para'} ${otra.alias}`;
}

export async function enviar(hoja) {
  const form = el('form', { clase: 'tarjeta' });
  const error = el('p', { clase: 'error', hidden: true });

  const destinatarios = el('select', { id: 'destino', required: true });
  destinatarios.append(el('option', { value: '', texto: 'Elegí a quién' }));

  const monto = el('input', {
    clase: 'importe-entrada', inputMode: 'decimal', placeholder: '$ 0,00',
    autocomplete: 'off', required: true,
  });

  const enviarBoton = el('button', { clase: 'boton boton-primario boton-ancho', type: 'submit', texto: 'Enviar dinero' });

  form.append(
    el('div', { clase: 'cabecera-tarjeta' }, el('h1', { texto: 'Enviar dinero' })),
    el('label', { clase: 'campo' }, el('span', { texto: 'Para' }), destinatarios),
    el('label', { clase: 'campo' }, el('span', { texto: 'Monto' }), monto),
    enviarBoton, error);
  hoja.append(form);

  olvidarAgenda();
  const contactos = await agenda();
  for (const c of contactos.values()) {
    if (c.id === cuentaID() || c.tipo === 'interna' || c.activa === false) continue;
    destinatarios.append(el('option', { value: String(c.id), texto: c.alias }));
  }
  if (destinatarios.options.length === 1) {
    destinatarios.append(el('option', { value: '', disabled: true, texto: 'No hay otras cuentas todavía' }));
  }

  const claveEnvio = claveDeIntento('envio');
  form.addEventListener('submit', async (ev) => {
    ev.preventDefault();
    error.hidden = true;

    const centavos = aCentavos(monto.value);
    if (!centavos) { mostrar(error, 'Escribí un monto válido'); return; }
    const destino = Number(destinatarios.value);
    if (!destino) { mostrar(error, 'Elegí a quién le enviás'); return; }

    enviarBoton.disabled = true;
    enviarBoton.textContent = 'Enviando…';
    try {
      const cuerpo = { origen_id: cuentaID(), destino_id: destino, monto_centavos: centavos };
      const r = await pedir('POST', '/transferencias', { clave: claveEnvio(cuerpo), cuerpo });
      const alias = destinatarios.selectedOptions[0]?.textContent || '';
      hoja.replaceChildren(resultado(r, {
        exito: `Le enviaste dinero a ${alias}`,
        centavos,
      }));
    } catch (err) {
      mostrar(error, err.message);
      enviarBoton.disabled = false;
      enviarBoton.textContent = 'Enviar dinero';
    }
  });
}

export async function retirar(hoja) {
  const form = el('form', { clase: 'tarjeta' });
  const error = el('p', { clase: 'error', hidden: true });
  const cbu = el('input', { placeholder: '0000003100010000000001', inputMode: 'numeric', required: true, autocomplete: 'off' });
  const monto = el('input', { clase: 'importe-entrada', inputMode: 'decimal', placeholder: '$ 0,00', required: true, autocomplete: 'off' });
  const accion = el('button', { clase: 'boton boton-primario boton-ancho', type: 'submit', texto: 'Retirar' });

  form.append(
    el('div', { clase: 'cabecera-tarjeta' }, el('h1', { texto: 'Retirar a una cuenta bancaria' })),
    el('label', { clase: 'campo' }, el('span', { texto: 'CBU o CVU de destino' }), cbu),
    el('label', { clase: 'campo' }, el('span', { texto: 'Monto' }), monto),
    accion, error);
  hoja.append(form);

  const claveRetiro = claveDeIntento('retiro');
  form.addEventListener('submit', async (ev) => {
    ev.preventDefault();
    error.hidden = true;
    const centavos = aCentavos(monto.value);
    if (!centavos) { mostrar(error, 'Escribí un monto válido'); return; }

    accion.disabled = true;
    accion.textContent = 'Procesando…';
    try {
      const cuerpo = { origen_id: cuentaID(), cbu_destino: cbu.value.trim(), monto_centavos: centavos };
      const r = await pedir('POST', '/egresos', { clave: claveRetiro(cuerpo), cuerpo });
      hoja.replaceChildren(resultado(r, { exito: 'Retiro enviado a tu banco', centavos }));
    } catch (err) {
      mostrar(error, err.message);
      accion.disabled = false;
      accion.textContent = 'Retirar';
    }
  });
}

export async function ingresar(hoja, ir) {
  const form = el('form', { clase: 'tarjeta' });
  const error = el('p', { clase: 'error', hidden: true });
  const monto = el('input', { clase: 'importe-entrada', inputMode: 'decimal', placeholder: '$ 0,00', required: true, autocomplete: 'off' });
  const accion = el('button', { clase: 'boton boton-primario boton-ancho', type: 'submit', texto: 'Continuar' });

  form.append(
    el('div', { clase: 'cabecera-tarjeta' }, el('h1', { texto: 'Ingresar dinero' })),
    el('label', { clase: 'campo' }, el('span', { texto: '¿Cuánto vas a transferir?' }), monto),
    accion, error);
  hoja.append(form);

  const claveIngreso = claveDeIntento('ingreso');
  form.addEventListener('submit', async (ev) => {
    ev.preventDefault();
    error.hidden = true;
    const centavos = aCentavos(monto.value);
    if (!centavos) { mostrar(error, 'Escribí un monto válido'); return; }

    accion.disabled = true;
    try {
      const cuerpo = { destino_id: cuentaID(), monto_centavos: centavos };
      const orden = await exigir('POST', '/ingresos', { clave: claveIngreso(cuerpo), cuerpo });
      hoja.replaceChildren(instruccionesDeIngreso(orden, centavos, ir));
    } catch (err) {
      mostrar(error, err.message);
      accion.disabled = false;
    }
  });
}

// "Ya hice la transferencia" hace de aviso del banco: la API registra el depósito en el
// proveedor vigente y concilia en la misma llamada.
function instruccionesDeIngreso(orden, centavos, ir) {
  const caja = el('section', { clase: 'tarjeta' });
  const estadoLinea = el('p', { clase: 'ayuda' });

  // El depósito se registra una sola vez; los reintentos sólo consultan la orden.
  let depositado = false;
  const buscar = async () => {
    if (depositado) return exigir('GET', `/ingresos/${orden.id}`);
    const r = await pedir('POST', `/ingresos/${orden.id}/simular-deposito`);
    if (r.codigo === 409 && r.datos?.codigo === 'ingreso_no_pendiente') {
      depositado = true;
      return exigir('GET', `/ingresos/${orden.id}`);
    }
    if (!r.ok) throw new Error(mensajeDeError(r));
    depositado = true;
    return r.datos;
  };

  const confirmar = el('button', {
    clase: 'boton boton-primario boton-ancho',
    texto: 'Ya hice la transferencia',
    onclick: async () => {
      confirmar.disabled = true;
      estadoLinea.textContent = 'Buscando tu transferencia…';
      try {
        const actualizada = await buscar();
        if (actualizada.estado === 'pendiente') {
          estadoLinea.textContent = 'Todavía no la vemos. Probá de nuevo en unos segundos.';
          confirmar.disabled = false;
        } else {
          caja.replaceChildren(...resultado({ ok: true, codigo: 200, datos: actualizada },
            { exito: 'Ya tenés el dinero en tu cuenta', centavos }).childNodes);
        }
      } catch (err) {
        estadoLinea.textContent = err.message;
        confirmar.disabled = false;
      }
    },
  });

  caja.append(
    el('div', { clase: 'cabecera-tarjeta' }, el('h1', { texto: 'Transferí desde tu banco' })),
    el('p', { clase: 'ayuda', texto: 'Usá este código como concepto para que podamos identificar tu transferencia.' }),
    el('div', { clase: 'rejilla' },
      dato('Código', orden.referencia),
      dato('Monto', enPesos(centavos))),
    el('div', { style: 'height:1rem' }),
    confirmar,
    estadoLinea,
    el('div', { clase: 'fila-botones', style: 'margin-top:.75rem' },
      el('button', { clase: 'enlace', texto: 'Volver al inicio', onclick: () => ir('#/inicio') })));
  return caja;
}

function dato(rotulo, valor) {
  return el('div', { clase: 'dato' },
    el('dt', { texto: rotulo }),
    el('dd', { clase: 'mono', texto: valor ?? '—' }));
}

export async function operacion(hoja, ir, tipo, id) {
  const ruta = { transferencia: 'transferencias', egreso: 'egresos', ingreso: 'ingresos' }[tipo] || 'transferencias';
  const o = await exigir('GET', `/${ruta}/${id}`);
  const d = desenlace(o.estado);
  const entra = o.destino_id === cuentaID();

  hoja.append(el('section', { clase: 'tarjeta' },
    el('div', { clase: 'resultado' },
      el('div', { clase: `sello sello-${d.tono}`, texto: d.tono === 'ok' ? '✓' : d.tono === 'no' ? '✕' : '…' }),
      el('h1', { texto: d.texto }),
      el('div', { clase: 'importe-grande', texto: conSigno(entra ? o.monto_centavos : -o.monto_centavos) })),
    el('div', { clase: 'rejilla' },
      dato('Número de operación', String(o.id)),
      dato('Fecha', fechaCompleta(o.creada_en)),
      o.referencia ? dato('Código', o.referencia) : null,
      o.cbu_destino ? dato('Destino', o.cbu_destino) : null),
    o.motivo ? el('p', { clase: 'ayuda', style: 'margin-top:.9rem', texto: o.motivo }) : null,
    el('div', { clase: 'fila-botones', style: 'margin-top:1rem' },
      el('button', { clase: 'boton', texto: 'Volver', onclick: () => ir('#/actividad') }),
      // La app no decide qué acciones existen: "resolver" sólo viene si la operación quedó en duda.
      o._links?.resolver
        ? el('button', {
            clase: 'boton boton-primario', texto: 'Actualizar estado',
            onclick: () => ir(`#/operacion/${tipo}/${id}`),
          })
        : null)));
}

// Un 202 es una operación despachada cuyo desenlace no se conoce: no se dice "listo" ni
// "error", se dice que se está procesando.
function resultado(r, { exito, centavos }) {
  const o = r.datos || {};
  const d = desenlace(o.estado);

  const enRevision = r.codigo === 202 || o.estado === 'en_duda';
  const bien = r.ok && !enRevision && d.tono === 'ok';

  const titulo = bien ? exito
    : enRevision ? 'Estamos procesando tu operación'
    : (o.motivo || (r.ok ? d.texto : mensajeDeError(r)));

  return el('section', { clase: 'tarjeta' },
    el('div', { clase: 'resultado' },
      el('div', {
        clase: `sello sello-${bien ? 'ok' : enRevision ? 'espera' : 'no'}`,
        texto: bien ? '✓' : enRevision ? '…' : '✕',
      }),
      el('h1', { texto: titulo }),
      centavos ? el('div', { clase: 'importe-grande', texto: enPesos(centavos) }) : null,
      o.id ? el('p', { clase: 'ayuda', texto: `Operación N.º ${o.id}` }) : null),
    el('div', { clase: 'fila-botones', style: 'justify-content:center' },
      el('button', {
        clase: 'boton boton-primario', texto: 'Listo',
        onclick: () => { location.hash = '#/inicio'; },
      }),
      o.id ? el('button', {
        clase: 'boton', texto: 'Ver detalle',
        onclick: () => { location.hash = `#/operacion/${o.tipo}/${o.id}`; },
      }) : null));
}

function mostrar(nodo, texto) {
  nodo.textContent = texto;
  nodo.hidden = false;
}
