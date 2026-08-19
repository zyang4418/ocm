import '@carbon/charts-react/styles.css'
import { Column, Grid, Tile } from '@carbon/react'
import { SimpleBarChart, LineChart } from '@carbon/charts-react'
import { useTranslation } from 'react-i18next'

// The app's brand color lives in --color-primary / --cds-link-color, but d3
// fills resolve to literal strings (no CSS var support), so the chart series
// pin the same hex here. Keep in sync with app.wxss / app.scss tokens.
const BRAND = '#2B5FF6'

// DashboardCharts renders the homepage's two @carbon/charts panels as one
// band. It is lazy-loaded from DashboardPage so the ~600KB d3/charts bundle
// stays out of the app's main chunk; each chart sizes to its holder container
// (fixed height in SCSS, width 100%). Empty prop arrays hide the panel, so
// the band degrades the same way the list sections do when the backend omits
// a field for a low-privilege user or an empty day.
export default function DashboardCharts({ periods, load, loadAll }) {
  const { t } = useTranslation('dashboard')

  // Series group names double as the color-scale keys, so the translated
  // string must be reused consistently for both data and options.
  const barGroup = t('charts.sessionCount')
  const lineGroup = loadAll ? t('charts.futureLoadAll') : t('charts.futureLoadMine')

  const barData = (periods ?? []).map((p) => ({
    group: barGroup,
    key: t('periodLabel.single', { period: p.period }),
    value: p.count,
  }))

  // Local-midnight Date objects: the time-scale x-axis needs real dates, and
  // pinning to the client's midnight keeps every point on its own calendar day.
  const lineData = (load ?? []).map((d) => ({
    group: lineGroup,
    date: new Date(`${d.date}T00:00:00`),
    value: d.count,
  }))

  const barOptions = {
    resizable: true,
    toolbar: { enabled: false },
    legend: { enabled: false }, // single series - the axis labels say it all
    color: { scale: { [barGroup]: BRAND } },
    axes: {
      left: { mapsTo: 'value', includeZero: true },
      bottom: { mapsTo: 'key', scaleType: 'labels' },
    },
  }

  const lineOptions = {
    resizable: true,
    toolbar: { enabled: false },
    legend: { enabled: false },
    color: { scale: { [lineGroup]: BRAND } },
    curve: 'curveMonotoneX',
    points: { radius: 3, enabled: true },
    axes: {
      left: { mapsTo: 'value', includeZero: true },
      bottom: { mapsTo: 'date', scaleType: 'time', primary: true },
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
