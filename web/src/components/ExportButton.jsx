import { useState } from 'react'
import { Button } from '@carbon/react'
import { Download } from '@carbon/icons-react'
import { useTranslation } from 'react-i18next'
import { useAuth } from '../auth/AuthContext.jsx'
import { apiDownload } from '../auth/api.js'

// ExportButton triggers an xlsx download from a GET export endpoint. It manages
// its own loading state (disabling the button while the request is in flight)
// and reports failures via the onError callback (typically the page's existing
// error notification) when provided, otherwise a thrown promise the caller can
// catch. Use it anywhere a contextual export is needed:
//   <ExportButton path="/api/classrooms/export" fallbackName="classrooms.xlsx" onError={setError} />
export default function ExportButton({
  path,
  fallbackName = 'export.xlsx',
  label,
  size = 'sm',
  onError,
  disabled = false,
  className,
}) {
  const { t } = useTranslation()
  const { token } = useAuth()
  const [exporting, setExporting] = useState(false)

  const displayLabel = label !== undefined ? label : t('action.export')

  const handleExport = async () => {
    try {
      setExporting(true)
      await apiDownload(path, { token, fallbackName })
    } catch (err) {
      if (onError) onError(err.message)
    } finally {
      setExporting(false)
    }
  }

  return (
    <Button
      kind="ghost"
      renderIcon={Download}
      size={size}
      className={className}
      onClick={handleExport}
      disabled={disabled || exporting}
    >
      {exporting ? t('action.exporting') : displayLabel}
    </Button>
  )
}
