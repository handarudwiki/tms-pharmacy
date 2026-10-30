// Categorical series colors in validated fixed order (identity, never cycled by rank).
export const seriesLight = ['#2a78d6', '#eb6834', '#1baf7a', '#eda100', '#e87ba4', '#008300', '#6250d6', '#e34948']
export const seriesDark = ['#3987e5', '#d95926', '#199e70', '#c98500', '#d55181', '#008300', '#9085e9', '#e66767']
export const seriesColor = (i: number, dark: boolean) => (dark ? seriesDark : seriesLight)[i % 8]
