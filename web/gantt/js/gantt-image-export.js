// ── Сохранение диаграммы Ганта картинкой (export-gantt-chart-image) ─────────
//
// Экспорт целиком клиентский (design.md Решение 1): берётся уже отрисованный
// живой SVG из #gantt-chart, приводится к самодостаточному виду (свой <style>
// с развёрнутыми токенами, залитый фон, нативная легенда, текст шкалы времени
// как SVG вместо исходных HTML-узлов) и из этого общего SVG-источника
// получаются оба формата — SVG отдаётся как есть, PNG растеризуется на canvas
// (design.md Решение 4).
//
// Важная находка задачи 3.1 (её нет в design.md буквально): шкала времени
// диаграммы (подписи месяцев/дней — `.upper-text`/`.lower-text`) в Frappe
// Gantt 1.2.2 рендерится HTML-блоком `.grid-header`, СОСЕДНИМ с `<svg
// class="gantt">`, а не его частью (сверено по факту по UMD-бандлу
// frappe-gantt@1.2.2 и через Playwright на реальном DOM — make_grid_header()
// добавляет `.grid-header` в `this.$container`, а не в `this.$svg`). Простое
// клонирование `<svg class="gantt">` дало бы картинку без подписей шкалы —
// прямое нарушение спеки «Картинка соответствует выбранному масштабу
// времени». Поэтому buildExportSvg() отдельно конвертирует текстовые узлы
// `.grid-header` в SVG `<text>` (см. buildHeaderTextGroup).

import { state } from './state.js';
import { showToast, showErrorModal, withSubmitLock } from './utils.js';

const SVG_NS = 'http://www.w3.org/2000/svg';

// Пределы холста браузера для растрового формата (design.md Решение 5,
// задача 3.6) — измерены ФАКТИЧЕСКИ в десктопном Chrome 150 через
// Playwright (canvas 2D, ctx.fillRect + toDataURL/toBlob): предел на каждую
// сторону — 65535px (65536 уже даёт пустой холст без исключения), предел
// площади — 16384×16384 = 268 435 456px² (16385×16384 уже пуст). Предел
// площади сработал раньше предела стороны на прямоугольнике, близком к
// квадрату — оба условия нужны, не только сторона. На мобильных устройствах
// пределы предположительно ниже (design.md, Риски) — актуальная проверка
// отложена на задачу 4.6, эти константы используются только на десктопе.
const MAX_CANVAS_DIMENSION_PX = 65535;
const MAX_CANVAS_AREA_PX = 16384 * 16384;

// Список CSS custom properties, от значений которых зависит вид диаграммы —
// и объявленных в gantt.css (переопределения тёмной темы Frappe Gantt), и
// в variables.css (палитра ролей/состояний). Значения читаются в момент
// экспорта через getComputedStyle(document.documentElement) — браузер уже
// резолвит цепочку var() до литерала, вручную дублировать хардкодом не
// нужно (design.md Решение 2: «токены обязательно развернуть»).
const TOKEN_NAMES = [
    '--g-header-background', '--g-row-color', '--g-row-border-color', '--g-border-color',
    '--g-tick-color', '--g-tick-color-thick', '--g-text-muted', '--g-text-dark', '--g-text-light',
    '--g-arrow-color', '--g-weekend-highlight-color', '--g-bar-color', '--g-bar-border', '--g-progress-color',
    '--g-handle-color',
    '--bg-primary', '--bg-tertiary',
    '--color-epic', '--color-epic-progress', '--color-story', '--color-story-progress',
    '--color-role-analyst', '--color-role-analyst-progress',
    '--color-role-be', '--color-role-be-progress',
    '--color-role-fe', '--color-role-fe-progress',
    '--color-role-mobile', '--color-role-mobile-progress',
    '--color-role-qa', '--color-role-qa-progress',
    '--color-role-it-leader', '--color-role-it-leader-progress',
    '--color-default', '--color-default-progress',
    '--color-success', '--color-warning', '--color-danger',
    '--text-muted', '--text-primary', '--text-secondary',
];

// Состав легенды продублирован из index.html (`.gantt-legend`,
// `index.html:186-198`) — design.md Решение 3, Риски: «Расхождение состава
// легенды между страницей и построителем картинки». При добавлении нового
// обозначения в интерфейс не забудь добавить его и сюда (приёмка — задача
// 4.5, ручная сверка состава).
const LEGEND_ITEMS = [
    { kind: 'solid', token: '--color-role-it-leader', label: 'IT-лидер' },
    { kind: 'solid', token: '--color-role-analyst', label: 'Аналитик' },
    { kind: 'solid', token: '--color-role-be', label: 'BE' },
    { kind: 'solid', token: '--color-role-fe', label: 'FE' },
    { kind: 'solid', token: '--color-role-mobile', label: 'Mobile' },
    { kind: 'solid', token: '--color-role-qa', label: 'QA' },
    { kind: 'completed', label: 'Завершено' },
    { kind: 'fact', label: 'Факт-маркер: раньше / позже плана' },
    { kind: 'no-assignee', label: 'Нет исполнителя' },
    { kind: 'pin-invalid', label: 'Закрепление недействительно' },
];

// initGanttImageExport — навешивает обработчики кнопок «🖼️ PNG»/«📐 SVG»
// (ux-brief.md раздел 3). Доступность самих кнопок переключается в
// gantt-renderer.js::renderGantt вместе с #task-count — единая точка
// истины «диаграмма есть/нет» (ux-brief.md раздел 5).
export function initGanttImageExport() {
    const btnPng = document.getElementById('btn-export-gantt-png');
    const btnSvg = document.getElementById('btn-export-gantt-svg');
    if (!btnPng || !btnSvg) return;

    btnPng.addEventListener('click', () => runExport(btnPng, [btnPng, btnSvg], exportGanttPng));
    btnSvg.addEventListener('click', () => runExport(btnSvg, [btnPng, btnSvg], exportGanttSvg));
}

// runExport — общая обвязка обратной связи и защиты от повторного запуска
// (ux-brief.md раздел 3), один в один с паттерном runRegenerateQuarter
// (app.js). Блокируются ОБЕ кнопки сразу — они делят общий источник
// (buildExportSvg), запуск второй подготовки параллельно с первой
// бессмысленен так же, как повторный клик по той же самой.
async function runExport(activeBtn, allBtns, action) {
    const originalLabel = activeBtn.innerHTML;
    await withSubmitLock(allBtns, async () => {
        activeBtn.innerHTML = '⏳ Подготовка…';
        try {
            const outcome = await action();
            // 'limit-exceeded' — PNG отказал по пределу холста (задача 3.6):
            // пользователь уже увидел объяснение в showErrorModal, тост с
            // «сохранено» здесь был бы ложным сообщением об успехе.
            if (outcome !== 'limit-exceeded') {
                showToast('Картинка сохранена', 'success');
            }
        } catch (err) {
            console.error('Не удалось подготовить картинку диаграммы Ганта:', err);
            showToast('Не удалось подготовить картинку. Попробуйте ещё раз.', 'error');
        } finally {
            activeBtn.innerHTML = originalLabel;
        }
    });
}

// exportGanttSvg — сохраняет собранный самодостаточный SVG как есть.
// Способ отдачи файла подтверждён задачей 1.1: Blob + временная <a download>
// (URL.createObjectURL), а не навигация на серверный эндпоинт — экспорт
// целиком клиентский (design.md Решение 1), сервера в цепочке нет.
async function exportGanttSvg() {
    const built = buildExportSvg();
    if (!built) throw new Error('gantt-export-no-diagram');

    const svgText = serializeSvg(built.svgEl);
    const blob = new Blob([svgText], { type: 'image/svg+xml;charset=utf-8' });
    downloadBlob(blob, buildExportFileName('svg'));
}

// exportGanttPng — растеризует тот же собранный SVG на canvas (design.md
// Решение 4: PNG — производная SVG, не вторая независимая реализация).
// Предел холста проверяется ДО создания canvas (design.md Решение 5, задача
// 3.6) — при превышении браузеры по-разному портят результат молча, поэтому
// заранее считаем размеры и отказываем с объяснением.
async function exportGanttPng() {
    const built = buildExportSvg();
    if (!built) throw new Error('gantt-export-no-diagram');

    const { svgEl, width, height } = built;
    if (width > MAX_CANVAS_DIMENSION_PX || height > MAX_CANVAS_DIMENSION_PX || width * height > MAX_CANVAS_AREA_PX) {
        showErrorModal(
            'Диаграмма в выбранном масштабе слишком велика для PNG — она не помещается в допустимый размер файла. ' +
            'Выберите более крупный масштаб времени (например, «Неделя» или «Месяц») либо сохраните диаграмму в SVG — ' +
            'для него это ограничение не действует.'
        );
        return 'limit-exceeded';
    }

    const svgText = serializeSvg(svgEl);
    const canvas = await rasterizeSvgToCanvas(svgText, width, height);
    const pngBlob = await canvasToBlob(canvas);
    downloadBlob(pngBlob, buildExportFileName('png'));
    return 'ok';
}

// ── Сборка самодостаточного SVG (задачи 3.1–3.3) ───────────────────────

// buildExportSvg — возвращает { svgEl, width, height } либо null, если
// диаграммы сейчас нет (защита в глубину — кнопки и так недоступны без
// диаграммы, ux-brief.md раздел 5).
function buildExportSvg() {
    const chartContainer = document.getElementById('gantt-chart');
    const liveSvg = chartContainer ? chartContainer.querySelector('svg.gantt') : null;
    const ganttContainerEl = chartContainer ? chartContainer.querySelector(':scope > .gantt-container') : null;
    if (!liveSvg || !ganttContainerEl) return null;

    const gridHeaderEl = ganttContainerEl.querySelector(':scope > .grid-header');

    // Ширина/высота из атрибутов живого SVG — уже посчитаны библиотекой на
    // ПОЛНЫЙ диапазон дат и ПОЛНОЕ число строк (design.md, Context: контейнер
    // прокручивается, а не сам SVG, — значит содержимое всего диапазона уже
    // в DOM без доскроллинга).
    const diagramWidth = Math.ceil(parseFloat(liveSvg.getAttribute('width')) || 0);
    const diagramHeight = Math.ceil(parseFloat(liveSvg.getAttribute('height')) || 0);
    if (!diagramWidth || !diagramHeight) return null;

    const tokens = resolveExportTokens();
    const legend = buildLegendGroup(tokens, diagramWidth);
    const headerHeight = gridHeaderEl ? gridHeaderEl.offsetHeight : 0;

    const totalWidth = diagramWidth;
    const totalHeight = legend.height + diagramHeight;

    const svgRoot = document.createElementNS(SVG_NS, 'svg');
    svgRoot.setAttribute('xmlns', SVG_NS);
    svgRoot.setAttribute('width', String(totalWidth));
    svgRoot.setAttribute('height', String(totalHeight));
    svgRoot.setAttribute('viewBox', `0 0 ${totalWidth} ${totalHeight}`);

    const styleEl = document.createElementNS(SVG_NS, 'style');
    styleEl.textContent = buildStyleText(tokens);
    svgRoot.appendChild(styleEl);

    // Фон на всю площадь картинки (design.md Решение 6, задача 3.2) — у SVG
    // нет фона по умолчанию, а диаграмма оформлена светлым по тёмному.
    svgRoot.appendChild(buildRect(0, 0, totalWidth, totalHeight, 'var(--bg-primary)'));

    // Легенда — над диаграммой, тем же порядком, что и на экране
    // (index.html: .gantt-legend расположена НАД #app-gantt-wrapper).
    svgRoot.appendChild(legend.group);

    const diagramGroup = document.createElementNS(SVG_NS, 'g');
    diagramGroup.setAttribute('transform', `translate(0, ${legend.height})`);

    // Фон полосы шкалы времени — под текстом дат, тон отличается от общего
    // фона (var(--g-header-background), как и на экране).
    if (headerHeight > 0) {
        diagramGroup.appendChild(buildRect(0, 0, diagramWidth, headerHeight, 'var(--g-header-background)'));
    }

    // Текст шкалы времени — см. комментарий в шапке файла: `.grid-header`
    // это HTML, а не часть исходного SVG, конвертируем точечно.
    if (gridHeaderEl) {
        diagramGroup.appendChild(buildHeaderTextGroup(gridHeaderEl, ganttContainerEl));
    }

    // Содержимое живого SVG (сетка, тики, стрелки зависимостей, бары со всеми
    // классами состояний — gantt-completed/gantt-no-assignee/gantt-pin-invalid
    // и факт-маркеры уже навешаны в DOM до клика на кнопку экспорта, см.
    // applyPostRenderEnhancements в gantt-renderer.js). Оборачиваем в
    // <g class="gantt">, а не второй вложенный <svg>, — тогда правила
    // .gantt-стилей ниже можно скопировать с селекторами как есть, без
    // переписывания под вложенный viewport.
    const ganttGroup = document.createElementNS(SVG_NS, 'g');
    ganttGroup.setAttribute('class', 'gantt');
    const clonedInner = liveSvg.cloneNode(true);
    // Frappe Gantt анимирует появление баров через SMIL <animate> (рост
    // width/height от 0 при первой отрисовке) — найдено при визуальной
    // проверке экспорта задачи 3.1, в design.md не описано. Эти узлы
    // клонируются вместе с остальным SVG и в СТАНДАЛОНЕ-файле проигрываются
    // заново при каждом открытии/растеризации: PNG — однокадровый снимок,
    // и без удаления анимации порядок «canvas нарисован ДО того, как
    // анимация докатилась до конечного состояния» дал бы недетерминированно
    // обрезанные бары; открытый как файл SVG на каждый показ заново играл
    // бы «вырастание» вместо статичной картинки. Убираем узлы анимации —
    // элементы остаются в их конечных (уже отрисованных) атрибутах x/y/
    // width/height, анимация просто не запускается.
    clonedInner.querySelectorAll('animate, animateTransform, animateMotion, animateColor, set')
        .forEach((el) => el.remove());
    while (clonedInner.firstChild) {
        ganttGroup.appendChild(clonedInner.firstChild);
    }
    diagramGroup.appendChild(ganttGroup);

    svgRoot.appendChild(diagramGroup);

    return { svgEl: svgRoot, width: totalWidth, height: totalHeight };
}

function buildRect(x, y, width, height, fill) {
    const rect = document.createElementNS(SVG_NS, 'rect');
    rect.setAttribute('x', String(x));
    rect.setAttribute('y', String(y));
    rect.setAttribute('width', String(width));
    rect.setAttribute('height', String(height));
    rect.setAttribute('fill', fill);
    return rect;
}

// resolveExportTokens — читает фактические (уже разрешённые браузером по
// цепочке var()) значения нужных custom properties с :root живого документа.
function resolveExportTokens() {
    const computed = getComputedStyle(document.documentElement);
    const tokens = {};
    TOKEN_NAMES.forEach((name) => {
        const value = computed.getPropertyValue(name).trim();
        if (value) tokens[name] = value;
    });
    return tokens;
}

// buildStyleText — правила скопированы по разделам источников (design.md
// Риски: «Перенесённый набор CSS-правил отстанет от реальных стилей» —
// копируем блоками, а не выборочными селекторами, чтобы новое правило
// оформления было проще не забыть перенести при следующей правке).
// Источник 1 — frappe-gantt@1.2.2 (frappe-gantt.min.css, версия закреплена
// в index.html). Источник 2 — gantt.css (секции «Custom Task Colors»,
// «Завершённые задачи», «Нет исполнителя/недействующее закрепление»,
// «Факт-маркер»). Значения var() резолвятся через :root, объявленный здесь
// же, внутри экспортируемого SVG (design.md Решение 2, второй вариант:
// «продублируй объявления внутри вставляемого блока»).
function buildStyleText(tokens) {
    const rootVars = Object.entries(tokens)
        .map(([name, value]) => `  ${name}: ${value};`)
        .join('\n');

    return `
:root {
${rootVars}
}

/* ── frappe-gantt@1.2.2, frappe-gantt.min.css (структурные правила) ── */
/* .grid-background — большой прямоугольник-подложка всей сетки (клик-зона
   библиотеки). Без явного fill:none у SVG-элементов начальное значение
   fill — чёрный, и этот прямоугольник, будучи позже .grid-row в разметке
   (выше по paint order), закрасил бы собой всю сетку сплошным чёрным —
   поймано на скриншоте экспортированного SVG при разработке задачи 3.1,
   не полагаться на «выглядит же нормально». */
.gantt .grid-background { fill: none; }
.gantt .grid-row { fill: var(--g-row-color); }
.gantt .row-line { stroke: var(--g-border-color); }
.gantt .tick { stroke: var(--g-tick-color); stroke-width: .4; }
.gantt .tick.thick { stroke: var(--g-tick-color-thick); stroke-width: .7; }
.gantt .arrow { fill: none; stroke: var(--g-arrow-color); stroke-width: 1.5; }
.gantt .bar-wrapper .bar { fill: var(--g-bar-color); stroke: var(--g-bar-border); stroke-width: 0; }
.gantt .bar-progress { fill: var(--g-progress-color); }
.gantt .bar-invalid { fill: transparent; stroke: var(--g-bar-border); stroke-width: 1; stroke-dasharray: 5; }
.gantt .bar-label { fill: var(--g-text-dark); dominant-baseline: central; font-family: Helvetica, Arial, sans-serif; font-size: 13px; font-weight: 400; }
.gantt .bar-label.big { fill: var(--g-text-dark); text-anchor: start; }
/* .handle — маркеры перетаскивания краёв бара, в состоянии покоя невидимы
   (opacity:0, видимы только на hover/active в интерактивном режиме, которых
   в статичной картинке не бывает). Без этого правила — та же ловушка, что
   и с .grid-background: элементы есть в DOM, default fill/opacity сделал бы
   их чёрными кружками поверх баров. */
.gantt .handle { fill: var(--g-handle-color); opacity: 0; }

/* ── gantt.css: Text Label Contrast Styles ── */
/* fix-gantt-telegram-rendering: stroke/paint-order убраны вслед за
   gantt.css — читаемость в экспорте теперь тоже держат непрозрачная
   подложка .bar-label-bg и инлайн fill на самом узле подписи
   (applyPostRenderEnhancements в gantt-renderer.js), которые cloneNode(true)
   копирует из живого DOM вместе с остальным SVG, а не это CSS-правило. */
.gantt .bar-label { fill: #ffffff !important; font-weight: 600; }
.gantt .bar-label.big { fill: var(--text-primary) !important; font-weight: 600; }
.gantt .bar-label-bg { fill: var(--bg-primary); pointer-events: none; }

/* ── gantt.css: Handles for dragging dates ── */
.gantt .handle-group .handle { fill: rgba(255, 255, 255, 0.35); }

/* ── gantt.css: Custom Task Colors based on Role ── */
.gantt .gantt-epic .bar { fill: var(--color-epic); fill-opacity: 1; stroke: none; rx: 6px; }
.gantt .gantt-epic .bar-progress { fill: var(--color-epic-progress); rx: 6px; }
.gantt .gantt-epic .bar-label { font-weight: 700; font-size: 13px; }

.gantt .gantt-story .bar { fill: var(--color-story); fill-opacity: 0.45; stroke: var(--color-story); stroke-width: 1.5px; rx: 4px; }
.gantt .gantt-story .bar-progress { fill: var(--color-story-progress); rx: 4px; }
.gantt .gantt-story .bar-label { font-weight: 600; font-size: 12px; }

.gantt .gantt-analyst .bar { fill: var(--color-role-analyst); }
.gantt .gantt-analyst .bar-progress { fill: var(--color-role-analyst-progress); }
.gantt .gantt-be .bar { fill: var(--color-role-be); }
.gantt .gantt-be .bar-progress { fill: var(--color-role-be-progress); }
.gantt .gantt-fe .bar { fill: var(--color-role-fe); }
.gantt .gantt-fe .bar-progress { fill: var(--color-role-fe-progress); }
.gantt .gantt-mobile .bar { fill: var(--color-role-mobile); }
.gantt .gantt-mobile .bar-progress { fill: var(--color-role-mobile-progress); }
.gantt .gantt-qa .bar { fill: var(--color-role-qa); }
.gantt .gantt-qa .bar-progress { fill: var(--color-role-qa-progress); }
.gantt .gantt-leader .bar { fill: var(--color-role-it-leader); }
.gantt .gantt-leader .bar-progress { fill: var(--color-role-it-leader-progress); }
.gantt .gantt-default .bar { fill: var(--color-default); }
.gantt .gantt-default .bar-progress { fill: var(--color-default-progress); }

/* ── gantt.css: Завершённые задачи ── */
.gantt .bar-wrapper.gantt-completed .bar { fill: var(--bg-tertiary); fill-opacity: 1; stroke: var(--color-success); stroke-width: 1.5px; opacity: 0.85; }
.gantt .bar-wrapper.gantt-completed .bar-progress { fill: var(--color-success); opacity: 0.9; }
.gantt .gantt-completed-icon { fill: var(--color-success); font-size: 12px; font-weight: 700; }

/* ── gantt.css: Нет исполнителя / недействующее закрепление ── */
.gantt .bar-wrapper.gantt-no-assignee:not(.gantt-completed) .bar { fill: var(--bg-tertiary); fill-opacity: 1; stroke: var(--text-muted); stroke-width: 1.5px; stroke-dasharray: 4 3; opacity: 0.6; }
.gantt .bar-wrapper.gantt-no-assignee:not(.gantt-completed) .bar-progress { fill: var(--text-muted); opacity: 0.5; }
.gantt .bar-wrapper.gantt-pin-invalid .bar { stroke: var(--color-warning); stroke-width: 2px; stroke-dasharray: 2 2; }
.gantt .gantt-pin-invalid-icon { fill: var(--color-warning); font-size: 12px; font-weight: 700; }

/* ── gantt.css: Факт-маркер ── */
.fact-marker { stroke-width: 2px; }
.fact-marker-diamond { stroke-width: 1px; }
.fact-marker.fact-marker-early, .fact-marker-diamond.fact-marker-early { stroke: var(--color-success); fill: var(--color-success); }
.fact-marker.fact-marker-late, .fact-marker-diamond.fact-marker-late { stroke: var(--color-danger); fill: var(--color-danger); }

/* ── Заголовок шкалы времени (сконвертирован из HTML, см. buildHeaderTextGroup)
   и легенда (нативные SVG-фигуры, design.md Решение 3) — своих правил в
   исходных CSS-файлах нет, стили заданы здесь по образцу соседних. ── */
.export-upper-text { fill: var(--g-text-dark); font-size: 14px; font-weight: 500; font-family: Helvetica, Arial, sans-serif; dominant-baseline: central; }
.export-lower-text { fill: var(--g-text-muted); font-size: 12px; font-weight: 400; font-family: Helvetica, Arial, sans-serif; dominant-baseline: central; }
.export-legend-label { fill: var(--text-secondary); font-size: 11px; font-family: Helvetica, Arial, sans-serif; }
.export-legend-icon-success { fill: var(--color-success); font-size: 10px; font-weight: 700; font-family: Helvetica, Arial, sans-serif; }
.export-legend-icon-warning { fill: var(--color-warning); font-size: 10px; font-weight: 700; font-family: Helvetica, Arial, sans-serif; }
`;
}

// ── Текст шкалы времени (задача 3.1) ────────────────────────────────────

// buildHeaderTextGroup — конвертирует `.upper-text`/`.lower-text` из живого
// `.grid-header` (HTML) в SVG `<text>`. Координаты берутся через offsetLeft/
// offsetTop-цепочку (getOffsetRelativeTo), а НЕ getBoundingClientRect: у
// `.grid-header` в Frappe Gantt `position: sticky` (frappe-gantt.min.css) —
// при вертикальной прокрутке диаграммы выше рабочей области его
// bounding-rect визуально «прилипает» к верху и не отражает истинное
// положение в полном диапазоне дат. offsetLeft/offsetTop — layout-свойства,
// не зависящие ни от прокрутки, ни от sticky-позиционирования (спека
// «Позиция прокрутки не влияет на результат»).
function buildHeaderTextGroup(gridHeaderEl, ganttContainerEl) {
    const group = document.createElementNS(SVG_NS, 'g');
    group.setAttribute('class', 'export-header-text');

    const nodes = gridHeaderEl.querySelectorAll('.upper-text, .lower-text');
    nodes.forEach((el) => {
        if (el.classList.contains('hide')) return;
        const text = (el.textContent || '').trim();
        if (!text) return;

        const offset = getOffsetRelativeTo(el, ganttContainerEl);
        const isUpper = el.classList.contains('upper-text');

        const textEl = document.createElementNS(SVG_NS, 'text');
        textEl.setAttribute('class', isUpper ? 'export-upper-text' : 'export-lower-text');
        if (isUpper) {
            // Подписи верхнего уровня (месяц/квартал) в исходной вёрстке
            // прижаты к левому краю своего периода (text-align не задан —
            // фактический text-anchor:start), кроме `.current-upper`,
            // которому на экране добавляется padding-left под sticky-кнопку
            // «Today» — в статичной картинке такой кнопки нет, поэтому
            // игнорируем этот частный случай намеренно, не переносим его.
            textEl.setAttribute('x', String(offset.x));
            textEl.setAttribute('text-anchor', 'start');
        } else {
            textEl.setAttribute('x', String(offset.x + el.offsetWidth / 2));
            textEl.setAttribute('text-anchor', 'middle');
        }
        textEl.setAttribute('y', String(offset.y + el.offsetHeight / 2));
        textEl.textContent = text;
        group.appendChild(textEl);
    });

    return group;
}

function getOffsetRelativeTo(el, ancestor) {
    let x = 0;
    let y = 0;
    let node = el;
    while (node && node !== ancestor) {
        x += node.offsetLeft || 0;
        y += node.offsetTop || 0;
        node = node.offsetParent;
    }
    return { x, y };
}

// ── Легенда (задача 3.3) ─────────────────────────────────────────────

const LEGEND_PADDING_X = 12;
const LEGEND_PADDING_Y = 10;
const LEGEND_GAP_ITEMS = 16;
const LEGEND_ROW_HEIGHT = 22;
const LEGEND_SWATCH = 14;
const LEGEND_FONT = '11px Helvetica, Arial, sans-serif';

let measureCtx = null;
function measureTextWidth(text) {
    if (!measureCtx) {
        measureCtx = document.createElement('canvas').getContext('2d');
    }
    measureCtx.font = LEGEND_FONT;
    return measureCtx.measureText(text).width;
}

// buildLegendGroup — рисует легенду прямоугольниками и текстом (design.md
// Решение 3: НЕ `<foreignObject>` — браузеры по-разному растеризуют его
// содержимое, вплоть до отказа рисовать). Перенос строк — простой ручной
// wrap по доступной ширине, имитирующий `flex-wrap` легенды на экране
// (gantt.css `.gantt-legend`), не обязан быть пиксель-в-пиксель.
function buildLegendGroup(tokens, availableWidth) {
    const group = document.createElementNS(SVG_NS, 'g');
    group.setAttribute('class', 'export-legend');
    group.setAttribute('transform', `translate(${LEGEND_PADDING_X}, ${LEGEND_PADDING_Y})`);

    const usableWidth = Math.max(availableWidth - LEGEND_PADDING_X * 2, LEGEND_SWATCH + 40);

    let x = 0;
    let y = 0;

    LEGEND_ITEMS.forEach((item) => {
        const swatchWidth = item.kind === 'fact' ? (8 * 2 + 2) : LEGEND_SWATCH;
        const itemWidth = swatchWidth + 6 + measureTextWidth(item.label);

        if (x > 0 && x + itemWidth > usableWidth) {
            x = 0;
            y += LEGEND_ROW_HEIGHT;
        }

        group.appendChild(buildLegendItem(item, x, y));
        x += itemWidth + LEGEND_GAP_ITEMS;
    });

    const rowsHeight = y + LEGEND_ROW_HEIGHT;
    return { group, height: rowsHeight + LEGEND_PADDING_Y * 2 };
}

function buildLegendItem(item, x, y) {
    const g = document.createElementNS(SVG_NS, 'g');
    g.setAttribute('transform', `translate(${x}, ${y})`);
    g.setAttribute('class', 'export-legend-item');

    const centerY = LEGEND_SWATCH / 2;

    if (item.kind === 'fact') {
        g.appendChild(buildCircle(4, centerY, 4, 'var(--color-success)'));
        g.appendChild(buildCircle(14, centerY, 4, 'var(--color-danger)'));
        g.appendChild(buildLegendLabel(item.label, 18 + 6, centerY));
        return g;
    }

    const rect = document.createElementNS(SVG_NS, 'rect');
    rect.setAttribute('width', String(LEGEND_SWATCH));
    rect.setAttribute('height', String(LEGEND_SWATCH));
    rect.setAttribute('rx', '3');

    let iconChar = '';
    let iconClass = '';
    if (item.kind === 'solid') {
        rect.setAttribute('fill', `var(${item.token})`);
    } else if (item.kind === 'completed') {
        rect.setAttribute('fill', 'var(--bg-tertiary)');
        rect.setAttribute('stroke', 'var(--color-success)');
        rect.setAttribute('stroke-width', '1.5');
        iconChar = '✓';
        iconClass = 'export-legend-icon-success';
    } else if (item.kind === 'no-assignee') {
        rect.setAttribute('fill', 'var(--bg-tertiary)');
        rect.setAttribute('stroke', 'var(--text-muted)');
        rect.setAttribute('stroke-width', '1.5');
        rect.setAttribute('stroke-dasharray', '3 2');
    } else if (item.kind === 'pin-invalid') {
        rect.setAttribute('fill', 'var(--bg-tertiary)');
        rect.setAttribute('stroke', 'var(--color-warning)');
        rect.setAttribute('stroke-width', '2');
        rect.setAttribute('stroke-dasharray', '2 2');
        iconChar = '⚠';
        iconClass = 'export-legend-icon-warning';
    }
    g.appendChild(rect);

    if (iconChar) {
        const iconText = document.createElementNS(SVG_NS, 'text');
        iconText.setAttribute('class', iconClass);
        iconText.setAttribute('x', String(LEGEND_SWATCH / 2));
        iconText.setAttribute('y', String(LEGEND_SWATCH / 2));
        iconText.setAttribute('text-anchor', 'middle');
        iconText.setAttribute('dominant-baseline', 'central');
        iconText.textContent = iconChar;
        g.appendChild(iconText);
    }

    g.appendChild(buildLegendLabel(item.label, LEGEND_SWATCH + 6, centerY));
    return g;
}

function buildCircle(cx, cy, r, fill) {
    const circle = document.createElementNS(SVG_NS, 'circle');
    circle.setAttribute('cx', String(cx));
    circle.setAttribute('cy', String(cy));
    circle.setAttribute('r', String(r));
    circle.setAttribute('fill', fill);
    return circle;
}

function buildLegendLabel(label, x, y) {
    const textEl = document.createElementNS(SVG_NS, 'text');
    textEl.setAttribute('class', 'export-legend-label');
    textEl.setAttribute('x', String(x));
    textEl.setAttribute('y', String(y));
    textEl.setAttribute('dominant-baseline', 'central');
    textEl.textContent = label;
    return textEl;
}

// ── Сериализация, растеризация, отдача файла (задачи 3.4–3.5) ──────────

function serializeSvg(svgEl) {
    const serialized = new XMLSerializer().serializeToString(svgEl);
    return `<?xml version="1.0" encoding="UTF-8"?>\n${serialized}`;
}

// rasterizeSvgToCanvas — грузит SVG как <img> через Blob-URL (тот же
// источник, что и для векторного формата — design.md Решение 4) и рисует на
// canvas без масштабирования (1:1, DPI/масштаб экспорта вне постановки —
// design.md Non-Goals). Blob-URL того же происхождения, что и документ, —
// canvas не «портится» (untainted), toBlob/toDataURL работают штатно.
function rasterizeSvgToCanvas(svgText, width, height) {
    const svgBlob = new Blob([svgText], { type: 'image/svg+xml;charset=utf-8' });
    const url = URL.createObjectURL(svgBlob);
    return loadImage(url)
        .then((img) => {
            const canvas = document.createElement('canvas');
            canvas.width = width;
            canvas.height = height;
            const ctx = canvas.getContext('2d');
            ctx.drawImage(img, 0, 0, width, height);
            return canvas;
        })
        .finally(() => URL.revokeObjectURL(url));
}

function loadImage(url) {
    return new Promise((resolve, reject) => {
        const img = new Image();
        img.onload = () => resolve(img);
        img.onerror = () => reject(new Error('gantt-export-svg-image-load-failed'));
        img.src = url;
    });
}

function canvasToBlob(canvas) {
    return new Promise((resolve, reject) => {
        canvas.toBlob((blob) => {
            if (blob) resolve(blob);
            else reject(new Error('gantt-export-canvas-to-blob-failed'));
        }, 'image/png');
    });
}

// downloadBlob — способ отдачи файла подтверждён задачей 1.1 фактической
// проверкой в десктопном Chrome через Playwright: Blob + временная
// `<a download>` с URL.createObjectURL надёжно инициирует сохранение,
// в отличие от навигации на серверный эндпоинт (которой здесь и нет —
// экспорт целиком клиентский).
function downloadBlob(blob, filename) {
    const url = URL.createObjectURL(blob);
    const link = document.createElement('a');
    link.href = url;
    link.download = filename;
    document.body.appendChild(link);
    link.click();
    link.remove();
    setTimeout(() => URL.revokeObjectURL(url), 1000);
}

// buildExportFileName — имя файла различает команду и момент сохранения
// (спека «Имена файлов различимы»), без совпадения с другими выгрузками
// приложения (сравни downloadReport в reports-panel.js).
function buildExportFileName(extension) {
    const teams = state.get('teams') || [];
    const teamId = state.get('selectedTeamId');
    const team = teams.find((t) => t.id === teamId);
    const teamSlug = slugify(team ? team.name : 'team');

    const now = new Date();
    const pad = (n) => String(n).padStart(2, '0');
    const stamp = `${now.getFullYear()}-${pad(now.getMonth() + 1)}-${pad(now.getDate())}_${pad(now.getHours())}-${pad(now.getMinutes())}`;

    return `gantt-${teamSlug}-${stamp}.${extension}`;
}

function slugify(text) {
    const slug = String(text)
        .trim()
        .toLowerCase()
        .replace(/[^a-zа-яё0-9]+/gi, '-')
        .replace(/-+/g, '-')
        .replace(/^-|-$/g, '');
    return slug || 'team';
}
