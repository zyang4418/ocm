/**
 * Weekly classroom-timetable grid (rowSpan merging via fixed-height rows).
 *
 * The page precomputes `rows` (one entry per period index, cells aligned with
 * `days`). A session block is rendered in the row where it starts, with
 * height = span * cellHeight, overflowing over the continuation rows below
 * (those render invisible spacers). Tap events bubble from the page.
 */
Component({
  properties: {
    days: { type: Array, value: [] },
    rows: { type: Array, value: [] },
    /** Row height in rpx; session blocks are span * cellHeight tall. */
    cellHeight: { type: Number, value: 96 },
    canManage: { type: Boolean, value: false }
  },
  methods: {
    onCellTap(e: WechatMiniprogram.TouchEvent) {
      if (!this.data.canManage) return
      const { date, periodindex, session } = e.currentTarget.dataset
      this.triggerEvent('celltap', {
        date,
        periodIndex: Number(periodindex),
        session: session === 'none' ? null : session
      })
    }
  }
})
