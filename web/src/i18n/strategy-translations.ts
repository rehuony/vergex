// Copy for the grid risk panel on the trader detail page.

export const gridRisk = {
  gridRisk: { zh: '网格风控', en: 'Grid Risk', es: 'Riesgo de Grid' },
  leverageInfo: { zh: '杠杆', en: 'Leverage', es: 'Apalancamiento' },
  positionInfo: { zh: '仓位', en: 'Position', es: 'Posición' },
  liquidationInfo: { zh: '清算', en: 'Liquidation', es: 'Liquidación' },
  marketState: { zh: '市场', en: 'Market', es: 'Mercado' },
  boxState: { zh: '箱体', en: 'Box', es: 'Caja' },
  currentLeverage: { zh: '当前', en: 'Current', es: 'Actual' },
  effectiveLeverage: { zh: '有效', en: 'Effective', es: 'Efectivo' },
  recommendedLeverage: { zh: '建议', en: 'Recommend', es: 'Recomendado' },
  currentPosition: { zh: '当前', en: 'Current', es: 'Actual' },
  maxPosition: { zh: '最大', en: 'Max', es: 'Máximo' },
  positionPercent: { zh: '占比', en: 'Usage', es: 'Uso' },
  liquidationPrice: { zh: '清算价', en: 'Liq Price', es: 'Precio Liquidación' },
  liquidationDistance: { zh: '距离', en: 'Distance', es: 'Distancia' },
  regimeLevel: { zh: '波动', en: 'Regime', es: 'Regulación' },
  currentPrice: { zh: '价格', en: 'Price', es: 'Precio' },
  breakoutLevel: { zh: '突破', en: 'Breakout', es: 'Breakout' },
  breakoutDirection: { zh: '方向', en: 'Direction', es: 'Dirección' },
  shortBox: { zh: '短期', en: 'Short', es: 'Corto' },
  midBox: { zh: '中期', en: 'Mid', es: 'Medio' },
  longBox: { zh: '长期', en: 'Long', es: 'Largo' },
  narrow: { zh: '窄幅', en: 'Narrow', es: 'Estrecho' },
  standard: { zh: '标准', en: 'Standard', es: 'Estándar' },
  wide: { zh: '宽幅', en: 'Wide', es: 'Ancho' },
  volatile: { zh: '剧烈', en: 'Volatile', es: 'Volátil' },
  trending: { zh: '趋势', en: 'Trending', es: 'Tendencia' },
  none: { zh: '无', en: 'None', es: 'Ninguno' },
  short: { zh: '短期', en: 'Short', es: 'Corto' },
  mid: { zh: '中期', en: 'Mid', es: 'Medio' },
  long: { zh: '长期', en: 'Long', es: 'Largo' },
  up: { zh: '↑', en: '↑', es: '↑' },
  down: { zh: '↓', en: '↓', es: '↓' },
  loading: { zh: '加载中...', en: 'Loading...', es: 'Cargando...' },
  error: { zh: '加载失败', en: 'Load Failed', es: 'Error al Cargar' },
  noData: { zh: '暂无数据', en: 'No Data', es: 'Sin Datos' },
}

export function ts(
  entry: { zh: string; en: string; [k: string]: string },
  lang: string
): string {
  return entry[lang] ?? entry.en ?? ''
}
