import { useEffect, useState } from 'react'
import { Button, InlineNotification, Modal, Select, SelectItem, TextInput } from '@carbon/react'
import { Calendar } from '@carbon/icons-react'
import { useTranslation } from 'react-i18next'
import { useAuth } from '../auth/AuthContext'
import { apiFetch } from '../auth/api'
import TimetablePickerModal, { type TimetablePick } from './TimetablePickerModal'
import type { Classroom, OfferingView, SessionInput, SessionView } from '../types/api'

export interface SessionFormModalProps {
  open: boolean
  /** Session being edited, or null for create mode. */
  target: SessionView | null
  /** Preselected offering for create mode (e.g. from an L2 expanded row). Not locked - the user may change it. */
  defaultOfferingId?: string
  offerings: OfferingView[]
  classrooms: Classroom[]
  /**
   * Reports the mutated offering plus the pre-edit offering when an edit moved
   * the session to a different offering, so callers can refresh every affected
   * cache (expanded-row caches and the sessions tab list).
   */
  onSuccess: (offeringId: number, prevOfferingId?: number) => void
  onClose: () => void
}

// Form state keeps ids/numbers as strings while editing; converted on submit.
interface SessionForm {
  offeringId: string
  classroomId: string
  date: string
  periodStart: string
  periodEnd: string
  note: string
}

const emptyForm: SessionForm = { offeringId: '', classroomId: '', date: '', periodStart: '', periodEnd: '', note: '' }

// SessionFormModal is the shared create/edit dialog for course sessions (L3),
// used by both the sessions tab and the offerings table's expanded rows. The
// frontend validates only presence and period ordering; period legality and
// classroom conflicts are enforced by the backend (400/409), whose message is
// shown inline while keeping the modal open - same layering as TimetablePage.
export default function SessionFormModal({
  open,
  target,
  defaultOfferingId,
  offerings,
  classrooms,
  onSuccess,
  onClose,
}: SessionFormModalProps) {
  const { t } = useTranslation('courses')
  const { token } = useAuth()
  const [form, setForm] = useState<SessionForm>(emptyForm)
  const [error, setError] = useState('')
  const [saving, setSaving] = useState(false)
  // Timetable picker layered on top of this dialog: picking a free cell
  // backfills classroom/date/periods here for review before submitting.
  // While it is open this form's Modal is closed: two stacked Carbon modals
  // keep two focus traps alive, and the lower one yanks focus back on every
  // focus move into the picker (a native <select> dropdown then closes
  // instantly). Form state lives here, not in the Modal children, so closing
  // the Modal loses nothing.
  const [pickerOpen, setPickerOpen] = useState(false)

  // (Re)initialize from the edit target / default offering each time the
  // dialog opens, mirroring how the page-level modals reset their forms.
  useEffect(() => {
    if (!open) return
    setForm(
      target
        ? {
            offeringId: String(target.offeringId),
            classroomId: String(target.classroomId),
            date: target.date,
            periodStart: String(target.periodStart),
            periodEnd: String(target.periodEnd),
            note: target.note,
          }
        : { ...emptyForm, offeringId: defaultOfferingId ?? '' },
    )
    setError('')
  }, [open, target, defaultOfferingId])

  const submit = async () => {
    if (!form.offeringId) {
      setError(t('sessionForm.selectOffering'))
      return
    }
    if (!form.classroomId) {
      setError(t('sessionForm.selectClassroom'))
      return
    }
    if (!form.date) {
      setError(t('sessionForm.dateRequired'))
      return
    }
    const periodStart = Number(form.periodStart)
    const periodEnd = form.periodEnd ? Number(form.periodEnd) : periodStart
    if (!periodStart || periodStart < 1 || periodEnd < periodStart) {
      setError(t('sessionForm.periodRange'))
      return
    }
    const body: SessionInput = {
      offeringId: Number(form.offeringId),
      classroomId: Number(form.classroomId),
      date: form.date,
      periodStart,
      periodEnd,
      note: form.note.trim(),
    }
    try {
      setSaving(true)
      setError('')
      if (target) {
        await apiFetch(`/api/sessions/${target.id}`, { method: 'PUT', token, body })
      } else {
        await apiFetch('/api/sessions', { method: 'POST', token, body })
      }
      const offeringId = Number(form.offeringId)
      onSuccess(offeringId, target && target.offeringId !== offeringId ? target.offeringId : undefined)
      onClose()
    } catch (err) {
      // 409 classroom conflict / 400 invalid period: keep the dialog open and
      // surface the backend message.
      setError((err as Error).message)
    } finally {
      setSaving(false)
    }
  }

  // Backfill a selection from the timetable picker: the classroom and the
  // full period range (which may differ from the session's current length)
  // come from the dragged cells. Everything stays editable below - the pick
  // is only a proposal until the form is submitted.
  const applyPick = ({ classroomId, date, periodStart, periodEnd }: TimetablePick) => {
    setForm((f) => ({ ...f, classroomId, date, periodStart: String(periodStart), periodEnd: String(periodEnd) }))
    setPickerOpen(false)
  }

  return (
    <>
      {/* open && !pickerOpen: see the pickerOpen comment above. */}
      <Modal
        open={open && !pickerOpen}
        modalHeading={target ? t('sessionModal.edit', { date: target.date }) : t('sessionModal.create')}
        primaryButtonText={t('modal.editSubmit')}
        secondaryButtonText={t('action.cancel', { ns: 'common' })}
        onRequestClose={onClose}
        onRequestSubmit={submit}
        primaryButtonDisabled={saving}
      >
        <div className="courses-page__form">
          <Select
            id="session-form-offering"
            labelText={t('sessionForm.offering')}
            value={form.offeringId}
            onChange={(e) => setForm({ ...form, offeringId: e.target.value })}
          >
            <SelectItem value="" text={t('sessionForm.selectOffering')} />
            {offerings.map((o) => (
              <SelectItem
                key={o.id}
                value={String(o.id)}
                text={[o.catalogName, o.teachingClassName, o.teacher].filter(Boolean).join(' / ')}
              />
            ))}
          </Select>
          <Select
            id="session-form-classroom"
            labelText={t('sessionForm.classroom')}
            value={form.classroomId}
            onChange={(e) => setForm({ ...form, classroomId: e.target.value })}
          >
            <SelectItem value="" text={t('sessionForm.selectClassroom')} />
            {classrooms.map((c) => (
              <SelectItem key={c.id} value={String(c.id)} text={c.name} />
            ))}
          </Select>
          <Button
            kind="secondary"
            size="sm"
            renderIcon={Calendar}
            className="session-form__pick"
            onClick={() => setPickerOpen(true)}
            disabled={classrooms.length === 0}
          >
            {t('sessionForm.pickFromTimetable')}
          </Button>
          <TextInput
            id="session-form-date"
            type="date"
            labelText={t('sessionForm.date')}
            value={form.date}
            onChange={(e) => setForm({ ...form, date: e.target.value })}
          />
          <TextInput
            id="session-form-period-start"
            type="number"
            min="1"
            labelText={t('sessionForm.periodStart')}
            value={form.periodStart}
            onChange={(e) => setForm({ ...form, periodStart: e.target.value })}
          />
          <TextInput
            id="session-form-period-end"
            type="number"
            min="1"
            labelText={t('sessionForm.periodEnd')}
            helperText={t('sessionForm.periodHint')}
            value={form.periodEnd}
            onChange={(e) => setForm({ ...form, periodEnd: e.target.value })}
          />
          <TextInput
            id="session-form-note"
            labelText={t('sessionForm.note')}
            value={form.note}
            onChange={(e) => setForm({ ...form, note: e.target.value })}
          />
          {error && <InlineNotification kind="error" title={t('error.save')} subtitle={error} lowContrast hideCloseButton />}
        </div>
      </Modal>
      <TimetablePickerModal
        open={pickerOpen}
        classrooms={classrooms}
        initialClassroomId={form.classroomId}
        initialDate={form.date}
        targetSession={target}
        onClose={() => setPickerOpen(false)}
        onPick={applyPick}
      />
    </>
  )
}
