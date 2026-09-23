/*
Copyright (C) 2025 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/

// 阶梯计费表达式（billing_expr）的展示用解析器。
// 只做尽力而为的静态解析：提取 tier("name", body) 档位、档位条件与线性系数，
// 系数单位为 USD / 1M tokens，fixed(n) 为 USD / 次。解析失败返回 null，永不抛出。

export const BILLING_MODE_TIERED_EXPR = 'tiered_expr';

// 变量展示顺序与中文标签（作为 i18n key 使用）
export const BILLING_EXPR_VAR_LABELS = {
  p: '输入',
  c: '输出',
  cr: '缓存读取',
  cc: '缓存创建',
  cc1h: '缓存创建(1h)',
  img: '图片输入',
  img_cr: '图片缓存读取',
  img_o: '图片输出',
  ai: '音频输入',
  ao: '音频输出',
};

export const BILLING_EXPR_VARS = Object.keys(BILLING_EXPR_VAR_LABELS);

export const isTieredBillingMode = (mode) => mode === BILLING_MODE_TIERED_EXPR;

const OPERATORS = [
  '**',
  '??',
  '||',
  '&&',
  '==',
  '!=',
  '<=',
  '>=',
  '<',
  '>',
  '+',
  '-',
  '*',
  '/',
  '%',
  '^',
  '!',
  '?',
  ':',
  '(',
  ')',
  ',',
  '[',
  ']',
];

const WORD_OPERATORS = { and: '&&', or: '||', not: '!' };

function tokenize(src) {
  const tokens = [];
  let i = 0;
  while (i < src.length) {
    const ch = src[i];
    if (/\s/.test(ch)) {
      i++;
      continue;
    }
    const start = i;
    if (/[0-9]/.test(ch) || (ch === '.' && /[0-9]/.test(src[i + 1] || ''))) {
      const m = /^(?:\d[\d_]*)?(?:\.\d[\d_]*)?(?:[eE][+-]?\d+)?/.exec(
        src.slice(i),
      );
      if (!m || !m[0]) throw new Error('bad number');
      i += m[0].length;
      tokens.push({
        type: 'num',
        value: parseFloat(m[0].replace(/_/g, '')),
        start,
        end: i,
      });
      continue;
    }
    if (ch === '"' || ch === "'" || ch === '`') {
      let value = '';
      i++;
      while (i < src.length && src[i] !== ch) {
        if (src[i] === '\\' && i + 1 < src.length) i++;
        value += src[i];
        i++;
      }
      if (i >= src.length) throw new Error('unterminated string');
      i++;
      tokens.push({ type: 'str', value, start, end: i });
      continue;
    }
    if (/[A-Za-z_$]/.test(ch)) {
      const m = /^[A-Za-z_$][\w$]*(?:\.[A-Za-z_$][\w$]*)*/.exec(src.slice(i));
      i += m[0].length;
      if (WORD_OPERATORS[m[0]]) {
        tokens.push({ type: 'op', value: WORD_OPERATORS[m[0]], start, end: i });
      } else if (m[0] === 'in') {
        tokens.push({ type: 'op', value: 'in', start, end: i });
      } else {
        tokens.push({ type: 'id', value: m[0], start, end: i });
      }
      continue;
    }
    const op = OPERATORS.find((o) => src.startsWith(o, i));
    if (!op) throw new Error(`unexpected char ${ch}`);
    i += op.length;
    tokens.push({ type: 'op', value: op, start, end: i });
  }
  return tokens;
}

const BINARY_PRECEDENCE = {
  '??': 1,
  '||': 2,
  '&&': 3,
  '==': 4,
  '!=': 4,
  '<': 4,
  '>': 4,
  '<=': 4,
  '>=': 4,
  in: 4,
  '+': 5,
  '-': 5,
  '*': 6,
  '/': 6,
  '%': 6,
  '**': 7,
  '^': 7,
};

function parse(src) {
  const tokens = tokenize(src);
  let pos = 0;
  const peek = () => tokens[pos];
  const isOp = (v) => peek()?.type === 'op' && peek().value === v;
  const expect = (v) => {
    if (!isOp(v)) throw new Error(`expected ${v}`);
    return tokens[pos++];
  };

  const parseTernary = () => {
    const cond = parseBinary(1);
    if (!isOp('?')) return cond;
    pos++;
    const then = parseTernary();
    expect(':');
    const otherwise = parseTernary();
    return {
      type: 'cond',
      cond,
      then,
      otherwise,
      start: cond.start,
      end: otherwise.end,
    };
  };

  const parseBinary = (minPrec) => {
    let left = parseUnary();
    for (;;) {
      const tok = peek();
      const prec =
        tok?.type === 'op' ? BINARY_PRECEDENCE[tok.value] : undefined;
      if (prec === undefined || prec < minPrec) return left;
      pos++;
      const right = parseBinary(prec + 1);
      left = {
        type: 'bin',
        op: tok.value,
        left,
        right,
        start: left.start,
        end: right.end,
      };
    }
  };

  const parseUnary = () => {
    const tok = peek();
    if (
      tok?.type === 'op' &&
      (tok.value === '-' || tok.value === '!' || tok.value === '+')
    ) {
      pos++;
      const arg = parseUnary();
      return {
        type: 'unary',
        op: tok.value,
        arg,
        start: tok.start,
        end: arg.end,
      };
    }
    return parsePrimary();
  };

  const parsePrimary = () => {
    const tok = tokens[pos++];
    if (!tok) throw new Error('unexpected end');
    if (tok.type === 'num' || tok.type === 'str') {
      return {
        type: tok.type,
        value: tok.value,
        start: tok.start,
        end: tok.end,
      };
    }
    if (tok.type === 'id') {
      if (isOp('(')) {
        pos++;
        const args = [];
        while (!isOp(')')) {
          args.push(parseTernary());
          if (!isOp(')')) expect(',');
        }
        const close = expect(')');
        return {
          type: 'call',
          name: tok.value,
          args,
          start: tok.start,
          end: close.end,
        };
      }
      return { type: 'id', name: tok.value, start: tok.start, end: tok.end };
    }
    if (tok.type === 'op' && tok.value === '(') {
      const inner = parseTernary();
      const close = expect(')');
      return { ...inner, start: tok.start, end: close.end, paren: true };
    }
    if (tok.type === 'op' && tok.value === '[') {
      const items = [];
      while (!isOp(']')) {
        items.push(parseTernary());
        if (!isOp(']')) expect(',');
      }
      const close = expect(']');
      return { type: 'array', items, start: tok.start, end: close.end };
    }
    throw new Error('unexpected token');
  };

  const ast = parseTernary();
  if (pos !== tokens.length) throw new Error('trailing tokens');
  return ast;
}

// 将节点化简为线性形式 { k: 常数, coef: {var: 系数}, fixed }；非线性返回 null
function linearize(node) {
  switch (node.type) {
    case 'num':
      return { k: node.value, coef: {}, fixed: null };
    case 'id':
      if (!BILLING_EXPR_VARS.includes(node.name)) return null;
      return { k: 0, coef: { [node.name]: 1 }, fixed: null };
    case 'unary': {
      if (node.op === '!') return null;
      const a = linearize(node.arg);
      if (!a) return null;
      return node.op === '-' ? scale(a, -1) : a;
    }
    case 'call': {
      if (node.name === 'fixed' && node.args.length === 1) {
        const a = linearize(node.args[0]);
        if (!a || Object.keys(a.coef).length > 0 || a.fixed !== null)
          return null;
        return { k: 0, coef: {}, fixed: a.k };
      }
      return null;
    }
    case 'bin': {
      const a = linearize(node.left);
      const b = linearize(node.right);
      if (!a || !b) return null;
      if (node.op === '+' || node.op === '-') {
        const sign = node.op === '-' ? -1 : 1;
        const coef = { ...a.coef };
        Object.entries(b.coef).forEach(([v, c]) => {
          coef[v] = (coef[v] || 0) + sign * c;
        });
        let fixed = a.fixed;
        if (b.fixed !== null) fixed = (fixed || 0) + sign * b.fixed;
        return { k: a.k + sign * b.k, coef, fixed };
      }
      if (node.op === '*') {
        if (isConst(a)) return scale(b, a.k);
        if (isConst(b)) return scale(a, b.k);
        return null;
      }
      if (node.op === '/' && isConst(b) && b.k !== 0) {
        return scale(a, 1 / b.k);
      }
      return null;
    }
    default:
      return null;
  }
}

const isConst = (l) => Object.keys(l.coef).length === 0 && l.fixed === null;

function scale(l, f) {
  const coef = {};
  Object.entries(l.coef).forEach(([v, c]) => {
    coef[v] = c * f;
  });
  return { k: l.k * f, coef, fixed: l.fixed === null ? null : l.fixed * f };
}

// 去掉浮点误差，例如 0.1 * 3 → 0.3
const clean = (n) => parseFloat(Number(n).toPrecision(12));

function buildTier(name, bodyNode, src, conditions) {
  const lin = linearize(bodyNode);
  const coefficients = {};
  if (lin) {
    BILLING_EXPR_VARS.forEach((v) => {
      if (lin.coef[v]) coefficients[v] = clean(lin.coef[v]);
    });
  }
  return {
    name,
    condition: conditions.length > 0 ? conditions.join(' && ') : null,
    coefficients,
    fixed: lin && lin.fixed !== null ? clean(lin.fixed) : null,
    constant: lin && lin.k ? clean(lin.k) : 0,
    linear: !!lin,
    raw: src.slice(bodyNode.start, bodyNode.end).trim(),
  };
}

function collectTiers(node, src, conditions, out) {
  if (!node) return;
  if (node.type === 'cond') {
    const condText = src.slice(node.cond.start, node.cond.end).trim();
    collectTiers(node.then, src, [...conditions, condText], out);
    collectTiers(node.otherwise, src, conditions, out);
    return;
  }
  if (node.type === 'call' && node.name === 'tier') {
    const [nameNode, body] = node.args;
    if (!body) return;
    const name = nameNode?.type === 'str' ? nameNode.value : '';
    out.push(buildTier(name, body, src, conditions));
    return;
  }
  // 其余节点（如请求倍率乘法）继续向下查找档位
  if (node.type === 'bin') {
    collectTiers(node.left, src, conditions, out);
    collectTiers(node.right, src, conditions, out);
  } else if (node.type === 'unary') {
    collectTiers(node.arg, src, conditions, out);
  } else if (node.type === 'call') {
    node.args.forEach((a) => collectTiers(a, src, conditions, out));
  }
}

/**
 * 解析阶梯计费表达式。
 * @param {string} expr 表达式字符串，可带 "v1:" 版本前缀
 * @returns {{version:number, expr:string, tiers:Array<{name:string, condition:string|null,
 *   coefficients:Object, fixed:number|null, constant:number, linear:boolean, raw:string}>}|null}
 */
export function parseBillingExpr(expr) {
  try {
    if (typeof expr !== 'string' || expr.trim() === '') return null;
    let version = 1;
    let body = expr.trim();
    const m = /^v(\d+):/.exec(body);
    if (m) {
      version = parseInt(m[1], 10);
      body = body.slice(m[0].length);
    }
    const ast = parse(body);
    const tiers = [];
    collectTiers(ast, body, [], tiers);
    if (tiers.length === 0) {
      // 无 tier() 的纯线性表达式，视为单一档位
      const single = buildTier('', ast, body, []);
      if (!single.linear) return null;
      tiers.push(single);
    }
    return { version, expr: body, tiers };
  } catch (e) {
    return null;
  }
}

// 在解析结果中按名称查找档位
export function findBillingTier(parsed, name) {
  if (!parsed || !Array.isArray(parsed.tiers)) return null;
  return parsed.tiers.find((tier) => tier.name === name) || null;
}
