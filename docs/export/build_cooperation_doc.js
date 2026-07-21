const fs = require("fs");
const {
  Document, Packer, Paragraph, TextRun, Table, TableRow, TableCell,
  AlignmentType, HeadingLevel, BorderStyle, WidthType, ShadingType,
  VerticalAlign, LevelFormat, Header, Footer, PageNumber,
} = require("docx");

// ---- 字体 / 颜色 ----
const BODY_FONT = "宋体";
const HEAD_FONT = "黑体";
const ACCENT = "1F4E79";   // 深蓝
const LIGHT = "D9E2F3";    // 浅蓝表头
const GREY = "F2F2F2";

const PAGE_W = 11906, PAGE_H = 16838, MARGIN = 1440;
const CONTENT_W = PAGE_W - MARGIN * 2; // 9026

const cellBorder = { style: BorderStyle.SINGLE, size: 1, color: "BFBFBF" };
const borders = { top: cellBorder, bottom: cellBorder, left: cellBorder, right: cellBorder };
const cellMargins = { top: 70, bottom: 70, left: 120, right: 120 };

function tcell(text, { w, fill, bold = false, color, align = AlignmentType.LEFT, lines } = {}) {
  let children;
  if (Array.isArray(lines)) {
    children = lines.map((ln) => new Paragraph({
      alignment: align,
      children: [new TextRun({ text: ln, bold, color, font: BODY_FONT, size: 20 })],
    }));
  } else {
    children = [new Paragraph({
      alignment: align,
      children: [new TextRun({ text, bold, color, font: BODY_FONT, size: 20 })],
    })];
  }
  return new TableCell({
    width: { size: w, type: WidthType.DXA },
    borders,
    margins: cellMargins,
    verticalAlign: VerticalAlign.CENTER,
    shading: fill ? { fill, type: ShadingType.CLEAR } : undefined,
    children,
  });
}

function headRow(cells, widths) {
  return new TableRow({
    tableHeader: true,
    children: cells.map((c, i) => tcell(c, { w: widths[i], fill: ACCENT, bold: true, color: "FFFFFF" })),
  });
}

function spacerPara() {
  return new Paragraph({ spacing: { after: 120 }, children: [new TextRun({ text: "" })] });
}

// ---------- 合作方式对比表 ----------
const W3 = [2200, 2200, 4626];
const methodTable = new Table({
  width: { size: CONTENT_W, type: WidthType.DXA },
  columnWidths: W3,
  rows: [
    headRow(["合作方式", "价格", "范围与说明"], W3),
    new TableRow({ children: [
      tcell("方式一\n纯买断源码", { w: W3[0], fill: GREY, bold: true, lines: ["方式一", "纯买断源码"] }),
      tcell("4 万（一次性）", { w: W3[1], bold: true }),
      tcell("", { w: W3[2], lines: [
        "• 仅当前版本代码快照",
        "• 不含更新、技术支持、踩坑兜底",
        "• 交付后风险自担（诚信承诺价 / 闲鱼参考）",
      ] }),
    ]}),
    new TableRow({ children: [
      tcell("", { w: W3[0], fill: GREY, bold: true, lines: ["方式二", "买断 + 技术支持"] }),
      tcell("20 – 30 万（总包）", { w: W3[1], bold: true }),
      tcell("", { w: W3[2], lines: [
        "• 代码交付 + 技术支持",
        "• 技术支持建议拆为按年续费",
        "• 不全部打包进一次性总包",
      ] }),
    ]}),
    new TableRow({ children: [
      tcell("", { w: W3[0], fill: LIGHT, bold: true, color: ACCENT, lines: ["方式三 ★ 主推", "平台服务型合作", "（保底 + 分成）"] }),
      tcell("", { w: W3[1], fill: LIGHT, bold: true, color: ACCENT, lines: ["保底 + 分成", "（见下方明细）"] }),
      tcell("", { w: W3[2], fill: LIGHT, lines: [
        "• 长期合作，利益绑定",
        "• 保底保成本，分成共做大流水",
        "• 详见下表",
      ] }),
    ]}),
  ],
});

// ---------- 方式三 明细表 ----------
const D2 = [2800, 6226];
const detailTable = new Table({
  width: { size: CONTENT_W, type: WidthType.DXA },
  columnWidths: D2,
  rows: [
    headRow(["项目", "内容"], D2),
    new TableRow({ children: [
      tcell("实施 / 接入费", { w: D2[0], fill: GREY, bold: true }),
      tcell("一次性 10 万（落点 5–10 万，可并入首年）", { w: D2[1] }),
    ]}),
    new TableRow({ children: [
      tcell("保底月费", { w: D2[0], fill: GREY, bold: true }),
      tcell("", { w: D2[1], lines: ["报价 18 万/月  |  落点 12–15 万/月  |  底线 10 万/月"] }),
    ]}),
    new TableRow({ children: [
      tcell("分成", { w: D2[0], fill: GREY, bold: true }),
      tcell("平台流水的 %，按月流水分档；待盘子数据确定（流水从平台过，可核可测）", { w: D2[1] }),
    ]}),
    new TableRow({ children: [
      tcell("团队投入（对客口径）", { w: D2[0], fill: GREY, bold: true }),
      tcell("", { w: D2[1], lines: [
        "5 人（1 运维 + 2 运营 + 2 研发），月成本 10–15 万",
        "3 人（1 运维 + 2 研发，运营客户自出），月成本 6–10 万",
      ] }),
    ]}),
    new TableRow({ children: [
      tcell("合同", { w: D2[0], fill: GREY, bold: true }),
      tcell("首期 1–2 年 + 中途解约赔付保底 N 个月", { w: D2[1] }),
    ]}),
  ],
});

// ---------- 分工表 ----------
const F2 = [2400, 6626];
const divisionTable = new Table({
  width: { size: CONTENT_W, type: WidthType.DXA },
  columnWidths: F2,
  rows: [
    headRow(["承担方", "职责"], F2),
    new TableRow({ children: [
      tcell("客户方", { w: F2[0], fill: GREY, bold: true }),
      tcell("运营商供给 + 客户 / 销售 + 业务运营", { w: F2[1] }),
    ]}),
    new TableRow({ children: [
      tcell("我方", { w: F2[0], fill: GREY, bold: true }),
      tcell("平台产品 + 运维 + 持续研发 + 计费保障（SLA）", { w: F2[1] }),
    ]}),
  ],
});

function bullet(text, ref = "bullets") {
  return new Paragraph({
    numbering: { reference: ref, level: 0 },
    spacing: { after: 60 },
    children: [new TextRun({ text, font: BODY_FONT, size: 21 })],
  });
}

function h1(text) {
  return new Paragraph({ heading: HeadingLevel.HEADING_1, spacing: { before: 280, after: 140 },
    children: [new TextRun({ text, font: HEAD_FONT, bold: true, color: ACCENT })] });
}

const doc = new Document({
  styles: {
    default: { document: { run: { font: BODY_FONT, size: 21 } } },
    paragraphStyles: [
      { id: "Heading1", name: "Heading 1", basedOn: "Normal", next: "Normal", quickFormat: true,
        run: { size: 28, bold: true, font: HEAD_FONT, color: ACCENT },
        paragraph: { spacing: { before: 280, after: 140 }, outlineLevel: 0 } },
    ],
  },
  numbering: {
    config: [
      { reference: "bullets", levels: [{ level: 0, format: LevelFormat.BULLET, text: "•",
        alignment: AlignmentType.LEFT, style: { paragraph: { indent: { left: 480, hanging: 260 } } } }] },
    ],
  },
  sections: [{
    properties: { page: { size: { width: PAGE_W, height: PAGE_H }, margin: { top: MARGIN, right: MARGIN, bottom: MARGIN, left: MARGIN } } },
    footers: { default: new Footer({ children: [new Paragraph({ alignment: AlignmentType.CENTER,
      children: [new TextRun({ text: "第 ", font: BODY_FONT, size: 18, color: "808080" }),
                 new TextRun({ children: [PageNumber.CURRENT], font: BODY_FONT, size: 18, color: "808080" }),
                 new TextRun({ text: " 页  |  商业机密 · 内部使用", font: BODY_FONT, size: 18, color: "808080" })] })] }) },
    children: [
      // 标题
      new Paragraph({ alignment: AlignmentType.CENTER, spacing: { after: 60 },
        children: [new TextRun({ text: "国产大模型 Token 平台合作方案", font: HEAD_FONT, bold: true, size: 40, color: ACCENT })] }),
      new Paragraph({ alignment: AlignmentType.CENTER, spacing: { after: 40 },
        children: [new TextRun({ text: "商务洽谈材料", font: BODY_FONT, size: 22, color: "595959" })] }),
      new Paragraph({ alignment: AlignmentType.CENTER, spacing: { after: 120 },
        children: [new TextRun({ text: "日期：2026-05-31     密级：商业机密", font: BODY_FONT, size: 18, color: "808080" })] }),
      new Paragraph({ border: { bottom: { style: BorderStyle.SINGLE, size: 8, color: ACCENT, space: 1 } }, children: [new TextRun({ text: "" })] }),

      h1("一、合作方式"),
      methodTable,
      spacerPara(),

      h1("二、方式三明细（平台服务型合作）"),
      detailTable,
      spacerPara(),

      h1("三、分工"),
      divisionTable,
      spacerPara(),

      h1("四、护城河条款（合同必写）"),
      bullet("三不交：不交全套运维 runbook；不交上游对接全流程知识；不交部署控制权"),
      bullet("代码：客户「用」，不「拥有」"),
      bullet("持续迭代——客户能 fork 的永远是落后版本"),

      h1("五、内部底线（不对外）"),
      bullet("纯源码买断：不低于 4 万"),
      bullet("买断 + 支持：不低于 20 万"),
      bullet("保底 < 10 万/月 → 不做"),
      bullet("分成：不签死任何百分比（待盘子确定）"),

      h1("六、今日必问"),
      bullet("盘子预估：终端客户数量、月流水 / GMV 量级（决定分成 %）"),
    ],
  }],
});

Packer.toBuffer(doc).then((buf) => {
  fs.writeFileSync("docs/export/合作方案.docx", buf);
  console.log("written: docs/export/合作方案.docx");
});
