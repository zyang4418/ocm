import { Pagination } from '@carbon/react'
import { paginationProps } from '../i18n/carbonLocale.js'

// ListPagination renders the pager for a server-paginated list. The displayed
// page is clamped to the last existing page (a delete can leave page pointing
// past the end). Carbon's onChange is routed to separate page-change /
// page-size-change callbacks — usePagedList resets to page 1 when the page
// size changes, so the two must not be conflated.
export default function ListPagination({
  page,
  pageSize,
  totalItems,
  onPageChange,
  onPageSizeChange,
  className,
}) {
  const lastPage = Math.max(1, Math.ceil(totalItems / pageSize))
  const shown = Math.min(page, lastPage)
  return (
    <Pagination
      className={className}
      page={shown}
      pageSize={pageSize}
      pageSizes={[100, 200, 500]}
      totalItems={totalItems}
      onChange={({ page: p, pageSize: s }) => {
        if (s !== pageSize) onPageSizeChange(s)
        else onPageChange(p)
      }}
      {...paginationProps()}
    />
  )
}
