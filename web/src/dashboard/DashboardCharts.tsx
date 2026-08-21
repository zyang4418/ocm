import '@carbon/charts-react/styles.css'
import { Column, Grid, Tile } from '@carbon/react'
import { SimpleBarChart, LineChart } from '@carbon/charts-react'
import { ScaleTypes } from '@carbon/charts'
import { useTranslation } from 'react-i18next'
import { useTheme } from '../theme/ThemeContext'
import type { DailyCount, DashboardPeriodCount } from '../types/api'

// d3 fills resolve to literal color strings (no CSS var support), so the chart
// series pin each Carbon theme's interactive color: #0f62fe on the light
// themes, and the lighter #4589ff on the dark themes for contrast.
//
// Charts also needs the theme passed via options.theme: it writes the value to
// data-carbon-theme on the chart holder, which scopes charts-specific tokens
// (--cds-grid-bg backdrop, the dataviz palettes, color-scheme). Those do NOT
// inherit from the cds--g100 class on <html>, so without this the backdrop
// stays white in every theme.
const SERIES_COLORS: Record<string, string> = { white: '#0f62fe', g10: '#0f62fe', g90: '#4589ff', g100: '#4589ff' }

interface DashboardChartsProps {
  periods: DashboardPeriodCount[]
  load: DailyCount[]
  loadAll: boolean
}

// DashboardCharts renders the homepage's two @carbon/charts panels as one
// band. It is lazy-loaded from DashboardPage so the ~600KB d3/charts bundle
// stays out of the app's main chunk; each chart sizes to its holder container
// (fixed height in SCSS, width 100%). Empty prop arrays hide the panel, so
// the band degrades the same way the list sections do when the backend omits
// a field for a low-privilege user or an empty day.
export default function DashboardCharts({ periods, load, loadAll }: DashboardChartsProps) {
  const { t } = useTranslation('dashboard')
  const { theme } = useTheme()
  const seriesColor = SERIES_COLORS[theme] ?? '#0f62fe'

  // Series group names double as the color-scale keys, so the translated
  // string must be reused consistently for both data and options.
  const barGroup = t('charts.sessionCount')
  const lineGroup = loadAll ? t('charts.futureLoadAll') : t('charts.futureLoadMine')

  const barData = periods.map((p) => ({
    group: barGroup,
    key: t('periodLabel.single', { period: p.period }),
    value: p.count,
  }))

  // Local-midnight Date objects: the time-scale x-axis needs real dates, and
  // pinning to the client's midnight keeps every point on its own calendar day.
  const lineData = load.map((d) => ({
    group: lineGroup,
    date: new Date(`${d.date}T00:00:00`),
    value: d.count,
  }))

  const barOptions = {
    resizable: true,
    theme,
    toolbar: { enabled: false },
    legend: { enabled: false }, // single series - the axis labels say it all
    color: { scale: { [barGroup]: seriesColor } },
    axes: {
      left: { mapsTo: 'value', includeZero: true },
      bottom: { mapsTo: 'key', scaleType: ScaleTypes.LABELS },
    },
  }

  const lineOptions = {
    resizable: true,
    theme,
    toolbar: { enabled: false },
    legend: { enabled: false },
    color: { scale: { [lineGroup]: seriesColor } },
    curve: 'curveMonotoneX',
    points: { radius: 3, enabled: true },
    axes: {
      left: { mapsTo: 'value', includeZero: true },
      bottom: { mapsTo: 'date', scaleType: ScaleTypes.TIME, primary: true },
    },
  }

  return (
    <Grid className="dashboard__charts">
      {barData.length > 0 && (
        <Column md={4} lg={8}>
          <Tile className="dashboard__panel dashboard__chart-panel">
            <div className="dashboard__panel-head">
              <h2 className="dashboard__panel-title">{t('charts.todayDistribution')}</h2>
            </div>
            <div className="dashboard__chart-holder">
              <SimpleBarChart data={barData} options={barOptions} />
            </div>
          </Tile>
        </Column>
      )}
      {lineData.length > 0 && (
        <Column md={4} lg={8}>
          <Tile className="dashboard__panel dashboard__chart-panel">
            <div className="dashboard__panel-head">
              <h2 className="dashboard__panel-title">
                {loadAll ? t('charts.futureLoadAll') : t('charts.futureLoadMine')}
              </h2>
            </div>
            <div className="dashboard__chart-holder">
              <LineChart data={lineData} options={lineOptions} />
            </div>
          </Tile>
        </Column>
      )}
    </Grid>
  )
}
