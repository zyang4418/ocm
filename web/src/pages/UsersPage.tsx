import { useState } from 'react'
import {
  Breadcrumb,
  BreadcrumbItem,
  Button,
  Checkbox,
  CheckboxGroup,
  Column,
  DataTable,
  DatePicker,
  DatePickerInput,
  Grid,
  InlineNotification,
  Modal,
  PasswordInput,
  Select,
  SelectItem,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableHeader,
  TableRow,
  TableToolbar,
  TableToolbarContent,
  TableToolbarSearch,
  Tag,
  TextInput,
  type DataTableHeader,
  type TagProps,
} from '@carbon/react'
import { Add, Edit, Password as PasswordIcon, TrashCan, UserSettings } from '@carbon/icons-react'
import { useNavigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { useAuth } from '../auth/AuthContext'
import { apiFetch } from '../auth/api'
import usePagedList from '../hooks/usePagedList'
import ListPagination from '../components/ListPagination'
import { formatDate } from '../i18n/formatters'
import { datePickerLocale } from '../i18n/carbonLocale'
import type {
  CatalogPermission,
  GroupBrief,
  Role,
  RoleBrief,
  User,
  UserGrantView,
  UserRolesInput,
  UserPermissionsInput,
} from '../types/api'

// User type enum values (not translatable text — labels come from i18n).
const TYPE_KEYS = ['student', 'teacher', 'staff'] as const

const TYPE_KIND: Record<string, TagProps<'div'>['type']> = {
  student: 'teal',
  teacher: 'blue',
  staff: 'gray',
}

const typeKind = (type: string): TagProps<'div'>['type'] => TYPE_KIND[type] ?? 'gray'

interface CreateForm {
  username: string
  password: string
  displayName: string
  type: string
}

const emptyCreate: CreateForm = { username: '', password: '', displayName: '', type: 'staff' }

// One grant checkbox row: a role with optional expiry, or a catalog permission.
interface RoleGrantEntry {
  checked: boolean
  expiresAt: string
}

// Grants modal state: one entry per role (with optional expiry) and one per
// catalog permission. Groups are display-only.
interface GrantForm {
  roles: Record<string, RoleGrantEntry>
  permissions: Record<string, boolean>
  groups: GroupBrief[]
}

const emptyGrantForm: GrantForm = { roles: {}, permissions: {}, groups: [] }

export default function UsersPage() {
  const { t } = useTranslation('users')
  const { token, user: currentUser, can } = useAuth()
  const canManage = can('user:manage')
  const navigate = useNavigate()

  const headers: DataTableHeader[] = [
    { key: 'id', header: t('field.id') },
    { key: 'username', header: t('field.username') },
    { key: 'displayName', header: t('field.displayName') },
    { key: 'type', header: t('field.type') },
    { key: 'roles', header: t('field.roles') },
    { key: 'groups', header: t('field.groups') },
    { key: 'createdAt', header: t('field.createdAt') },
  ]

  const list = usePagedList<User>({ path: '/api/users', token })
  const { loading } = list

  // Carbon DataTable keys rows by a string id.
  const tableRows = list.items.map((u) => ({ ...u, id: String(u.id) }))

  const [createOpen, setCreateOpen] = useState(false)
  const [createForm, setCreateForm] = useState<CreateForm>(emptyCreate)
  const [createError, setCreateError] = useState('')
  const [creating, setCreating] = useState(false)

  const [editTarget, setEditTarget] = useState<User | null>(null)
  const [editForm, setEditForm] = useState({ displayName: '', type: 'staff' })
  const [editError, setEditError] = useState('')
  const [editing, setEditing] = useState(false)

  const [pwdTarget, setPwdTarget] = useState<User | null>(null)
  const [pwdForm, setPwdForm] = useState({ password: '', confirm: '' })
  const [pwdError, setPwdError] = useState('')
  const [pwdSaving, setPwdSaving] = useState(false)

  const [deleteTarget, setDeleteTarget] = useState<User | null>(null)
  const [deleteError, setDeleteError] = useState('')
  const [deleting, setDeleting] = useState(false)

  const [grantTarget, setGrantTarget] = useState<User | null>(null)
  const [grantRoles, setGrantRoles] = useState<Role[]>([])
  const [grantCatalog, setGrantCatalog] = useState<CatalogPermission[]>([])
  const [grantForm, setGrantForm] = useState<GrantForm>(emptyGrantForm)
  const [grantError, setGrantError] = useState('')
  const [grantLoading, setGrantLoading] = useState(false)
  const [grantSaving, setGrantSaving] = useState(false)

  const handleCreate = async () => {
    const { username, password, displayName, type } = createForm
    if (!username.trim() || !password || !displayName.trim()) {
      setCreateError(t('validation.createRequired'))
      return
    }
    try {
      setCreating(true)
      setCreateError('')
      await apiFetch('/api/users', {
        method: 'POST',
        token,
        body: {
          username: username.trim(),
          password,
          displayName: displayName.trim(),
          type,
        },
      })
      setCreateOpen(false)
      list.reload()
    } catch (err) {
      setCreateError((err as Error).message)
    } finally {
      setCreating(false)
    }
  }

  const openEdit = (u: User) => {
    setEditTarget(u)
    setEditForm({ displayName: u.displayName, type: u.type })
    setEditError('')
  }

  const handleEdit = async () => {
    if (!editTarget) return
    if (!editForm.displayName.trim()) {
      setEditError(t('validation.displayNameRequired'))
      return
    }
    try {
      setEditing(true)
      setEditError('')
      await apiFetch(`/api/users/${editTarget.id}`, {
        method: 'PUT',
        token,
        body: {
          displayName: editForm.displayName.trim(),
          type: editForm.type,
        },
      })
      setEditTarget(null)
      list.reload()
    } catch (err) {
      setEditError((err as Error).message)
    } finally {
      setEditing(false)
    }
  }

  const openPassword = (u: User) => {
    setPwdTarget(u)
    setPwdForm({ password: '', confirm: '' })
    setPwdError('')
  }

  const handlePassword = async () => {
    if (!pwdTarget) return
    if (!pwdForm.password) {
      setPwdError(t('validation.passwordRequired'))
      return
    }
    if (pwdForm.password !== pwdForm.confirm) {
      setPwdError(t('validation.passwordMismatch'))
      return
    }
    try {
      setPwdSaving(true)
      setPwdError('')
      await apiFetch(`/api/users/${pwdTarget.id}/password`, {
        method: 'PATCH',
        token,
        body: { password: pwdForm.password },
      })
      setPwdTarget(null)
    } catch (err) {
      setPwdError((err as Error).message)
    } finally {
      setPwdSaving(false)
    }
  }

  const openDelete = (u: User) => {
    setDeleteTarget(u)
    setDeleteError('')
  }

  const handleDelete = async () => {
    if (!deleteTarget) return
    try {
      setDeleting(true)
      setDeleteError('')
      await apiFetch(`/api/users/${deleteTarget.id}`, { method: 'DELETE', token })
      setDeleteTarget(null)
      list.reload()
    } catch (err) {
      setDeleteError((err as Error).message)
    } finally {
      setDeleting(false)
    }
  }

  // ---- Grants modal ----

  const openGrants = async (u: User) => {
    setGrantTarget(u)
    setGrantError('')
    setGrantLoading(true)
    try {
      const [grants, roles, catalog] = await Promise.all([
        apiFetch<UserGrantView>(`/api/users/${u.id}/grants`, { token }),
        apiFetch<Role[]>('/api/roles', { token }),
        apiFetch<CatalogPermission[]>('/api/permissions', { token }),
      ])
      setGrantRoles(roles)
      setGrantCatalog(catalog)
      const form: GrantForm = { roles: {}, permissions: {}, groups: grants.groups }
      for (const role of roles) {
        const existing = grants.roles.find((g) => g.code === role.code)
        form.roles[role.code] = existing ? { checked: true, expiresAt: toDateInput(existing.expiresAt) } : { checked: false, expiresAt: '' }
      }
      for (const perm of catalog) {
        form.permissions[perm.code] = grants.permissions.some((g) => g.permission === perm.code)
      }
      setGrantForm(form)
    } catch (err) {
      setGrantError((err as Error).message)
    } finally {
      setGrantLoading(false)
    }
  }

  const handleGrantSave = async () => {
    if (!grantTarget) return
    try {
      setGrantSaving(true)
      setGrantError('')
      // Optional expiresAt: undefined drops the key on JSON.stringify, which
      // the Go side decodes as a nil pointer (never expires) — same wire
      // behavior as the explicit null the JS version sent.
      const roles: UserRolesInput['roles'] = Object.entries(grantForm.roles)
        .filter(([, v]) => v.checked)
        .map(([code, v]) => ({
          roleCode: code,
          expiresAt: v.expiresAt ? localMidnightUTC(v.expiresAt) : undefined,
        }))
      const permissions: UserPermissionsInput['permissions'] = Object.entries(grantForm.permissions)
        .filter(([, checked]) => checked)
        .map(([permission]) => ({ permission }))
      await apiFetch(`/api/users/${grantTarget.id}/roles`, {
        method: 'PUT',
        token,
        body: { roles },
      })
      await apiFetch(`/api/users/${grantTarget.id}/permissions`, {
        method: 'PUT',
        token,
        body: { permissions },
      })
      setGrantTarget(null)
      list.reload()
    } catch (err) {
      setGrantError((err as Error).message)
    } finally {
      setGrantSaving(false)
    }
  }

  const isSelf = (u: User | null | undefined) => Boolean(u) && String(u!.id) === String(currentUser?.id)

  const catalogGroups = groupCatalog(grantCatalog)

  return (
    <Grid fullWidth className="users-page">
      <Column sm={4} md={8} lg={16}>
        <Breadcrumb noTrailingSlash aria-label={t('aria.breadcrumb', { ns: 'common' })}>
          <BreadcrumbItem
            href="/"
            onClick={(e) => {
              e.preventDefault()
              navigate('/')
            }}
          >
            {t('breadcrumb.home')}
          </BreadcrumbItem>
          <BreadcrumbItem isCurrentPage>{t('breadcrumb.current')}</BreadcrumbItem>
        </Breadcrumb>
        <h1 className="users-page__heading">{t('title')}</h1>
        <p className="users-page__subtitle">{t('subtitle')}</p>
      </Column>

      <Column sm={4} md={8} lg={16}>
        {list.error && (
          <InlineNotification
            kind="error"
            title={t('error.load')}
            subtitle={list.error}
            lowContrast
            hideCloseButton
            className="users-page__notice"
          />
        )}

        <DataTable rows={tableRows} headers={headers}>
          {({
            rows,
            headers: tableHeaders,
            getTableProps,
            getHeaderProps,
            getRowProps,
            getToolbarProps,
          }) => (
            <TableContainer title={t('table.title')} description={t('table.description', { count: list.total })}>
              <TableToolbar {...getToolbarProps()}>
                <TableToolbarContent>
                  <TableToolbarSearch value={list.q} onChange={(e, v) => list.setQ(v ?? '')} placeholder={t('searchPlaceholder')} />
                  {canManage && (
                    <Button renderIcon={Add} size="sm" onClick={() => setCreateOpen(true)}>
                      {t('addButton')}
                    </Button>
                  )}
                </TableToolbarContent>
              </TableToolbar>
              <Table {...getTableProps()}>
                <TableHead>
                  <TableRow>
                    {tableHeaders.map((header) => (
                      <TableHeader {...getHeaderProps({ header })}>
                        {header.header}
                      </TableHeader>
                    ))}
                    <TableHeader>{t('field.actions')}</TableHeader>
                  </TableRow>
                </TableHead>
                <TableBody>
                  {loading ? (
                    <TableRow>
                      <TableCell colSpan={headers.length + 1}>{t('empty.loading')}</TableCell>
                    </TableRow>
                  ) : rows.length === 0 ? (
                    <TableRow>
                      <TableCell colSpan={headers.length + 1}>
                        {list.q ? t('empty.search') : t('empty.none')}
                      </TableCell>
                    </TableRow>
                  ) : (
                    rows.map((row) => {
                      const u = list.items.find((x) => String(x.id) === String(row.id))
                      if (!u) return null
                      return (
                        <TableRow {...getRowProps({ row })}>
                          {row.cells.map((cell) => {
                            if (cell.info.header === 'type') {
                              const value = cell.value as string
                              return (
                                <TableCell key={cell.id}>
                                  <Tag type={typeKind(value)} size="sm">
                                    {t('type.' + value, { defaultValue: value })}
                                  </Tag>
                                </TableCell>
                              )
                            }
                            if (cell.info.header === 'roles' || cell.info.header === 'groups') {
                              const items = (cell.value ?? []) as Array<RoleBrief | GroupBrief>
                              return (
                                <TableCell key={cell.id}>
                                  {items.length === 0 ? (
                                    '-'
                                  ) : (
                                    <div className="users-page__tags">
                                      {items.map((item) => (
                                        <Tag key={`${cell.info.header}-${item.id}`} type="cool-gray" size="sm">
                                          {item.name}
                                        </Tag>
                                      ))}
                                    </div>
                                  )}
                                </TableCell>
                              )
                            }
                            if (cell.info.header === 'createdAt') {
                              return <TableCell key={cell.id}>{formatDate(cell.value as string)}</TableCell>
                            }
                            return <TableCell key={cell.id}>{cell.value as string}</TableCell>
                          })}
                          <TableCell>
                            <div className="users-page__actions">
                              {canManage && (
                                <>
                                  <Button
                                    kind="ghost"
                                    size="sm"
                                    hasIconOnly
                                    renderIcon={Edit}
                                    iconDescription={t('action.edit', { ns: 'common' })}
                                    onClick={() => openEdit(u)}
                                  />
                                  <Button
                                    kind="ghost"
                                    size="sm"
                                    hasIconOnly
                                    renderIcon={PasswordIcon}
                                    iconDescription={t('iconAction.password')}
                                    onClick={() => openPassword(u)}
                                  />
                                  <Button
                                    kind="ghost"
                                    size="sm"
                                    hasIconOnly
                                    renderIcon={UserSettings}
                                    iconDescription={t('iconAction.grants')}
                                    onClick={() => openGrants(u)}
                                  />
                                  <Button
                                    kind="ghost"
                                    size="sm"
                                    hasIconOnly
                                    renderIcon={TrashCan}
                                    iconDescription={t('action.delete', { ns: 'common' })}
                                    disabled={isSelf(u)}
                                    onClick={() => openDelete(u)}
                                  />
                                </>
                              )}
                              {!canManage && <span className="users-page__readonly">{t('readonly')}</span>}
                            </div>
                          </TableCell>
                        </TableRow>
                      )
                    })
                  )}
                </TableBody>
              </Table>
            </TableContainer>
          )}
        </DataTable>
        <ListPagination
          page={list.page}
          pageSize={list.pageSize}
          totalItems={list.total}
          onPageChange={list.setPage}
          onPageSizeChange={list.setPageSize}
        />
      </Column>

      {/* Create */}
      <Modal
        open={createOpen}
        modalHeading={t('modal.create')}
        primaryButtonText={t('modal.createSubmit')}
        secondaryButtonText={t('action.cancel', { ns: 'common' })}
        onRequestClose={() => setCreateOpen(false)}
        onRequestSubmit={handleCreate}
        primaryButtonDisabled={creating}
      >
        <div className="users-page__form">
          <TextInput
            id="create-username"
            labelText={t('form.username')}
            value={createForm.username}
            onChange={(e) => setCreateForm({ ...createForm, username: e.target.value })}
          />
          <PasswordInput
            id="create-password"
            labelText={t('form.password')}
            placeholder={t('form.passwordPlaceholder')}
            value={createForm.password}
            onChange={(e) => setCreateForm({ ...createForm, password: e.target.value })}
            showPasswordLabel={t('password.show', { ns: 'common' })}
            hidePasswordLabel={t('password.hide', { ns: 'common' })}
          />
          <TextInput
            id="create-displayName"
            labelText={t('form.displayName')}
            value={createForm.displayName}
            onChange={(e) => setCreateForm({ ...createForm, displayName: e.target.value })}
          />
          <Select
            id="create-type"
            labelText={t('form.type')}
            value={createForm.type}
            onChange={(e) => setCreateForm({ ...createForm, type: e.target.value })}
          >
            {TYPE_KEYS.map((k) => (
              <SelectItem key={k} value={k} text={t('type.' + k)} />
            ))}
          </Select>
          {createError && (
            <InlineNotification
              kind="error"
              title={t('error.create')}
              subtitle={createError}
              lowContrast
              hideCloseButton
            />
          )}
        </div>
      </Modal>

      {/* Edit */}
      <Modal
        open={Boolean(editTarget)}
        modalHeading={t('modal.edit', { name: editTarget?.username ?? '' })}
        primaryButtonText={t('modal.editSubmit')}
        secondaryButtonText={t('action.cancel', { ns: 'common' })}
        onRequestClose={() => setEditTarget(null)}
        onRequestSubmit={handleEdit}
        primaryButtonDisabled={editing}
      >
        <div className="users-page__form">
          <TextInput
            id="edit-displayName"
            labelText={t('form.displayName')}
            value={editForm.displayName}
            onChange={(e) => setEditForm({ ...editForm, displayName: e.target.value })}
          />
          <Select
            id="edit-type"
            labelText={t('form.type')}
            value={editForm.type}
            onChange={(e) => setEditForm({ ...editForm, type: e.target.value })}
          >
            {TYPE_KEYS.map((k) => (
              <SelectItem key={k} value={k} text={t('type.' + k)} />
            ))}
          </Select>
          {editError && (
            <InlineNotification
              kind="error"
              title={t('error.save')}
              subtitle={editError}
              lowContrast
              hideCloseButton
            />
          )}
        </div>
      </Modal>

      {/* Password */}
      <Modal
        open={Boolean(pwdTarget)}
        modalHeading={t('modal.password', { name: pwdTarget?.username ?? '' })}
        primaryButtonText={t('modal.passwordSubmit')}
        secondaryButtonText={t('action.cancel', { ns: 'common' })}
        onRequestClose={() => setPwdTarget(null)}
        onRequestSubmit={handlePassword}
        primaryButtonDisabled={pwdSaving}
      >
        <div className="users-page__form">
          <PasswordInput
            id="pwd-password"
            labelText={t('form.newPassword')}
            value={pwdForm.password}
            onChange={(e) => setPwdForm({ ...pwdForm, password: e.target.value })}
            showPasswordLabel={t('password.show', { ns: 'common' })}
            hidePasswordLabel={t('password.hide', { ns: 'common' })}
          />
          <PasswordInput
            id="pwd-confirm"
            labelText={t('form.confirmPassword')}
            value={pwdForm.confirm}
            onChange={(e) => setPwdForm({ ...pwdForm, confirm: e.target.value })}
            showPasswordLabel={t('password.show', { ns: 'common' })}
            hidePasswordLabel={t('password.hide', { ns: 'common' })}
          />
          {pwdError && (
            <InlineNotification
              kind="error"
              title={t('error.password')}
              subtitle={pwdError}
              lowContrast
              hideCloseButton
            />
          )}
        </div>
      </Modal>

      {/* Grants */}
      <Modal
        open={Boolean(grantTarget)}
        modalHeading={t('modal.grants', { name: grantTarget?.displayName ?? '' })}
        primaryButtonText={t('modal.grantsSubmit')}
        secondaryButtonText={t('action.cancel', { ns: 'common' })}
        onRequestClose={() => setGrantTarget(null)}
        onRequestSubmit={handleGrantSave}
        primaryButtonDisabled={grantSaving}
        size="lg"
      >
        <div className="users-page__grants">
          {grantLoading && <p>{t('empty.loading')}</p>}
          {grantError && (
            <InlineNotification
              kind="error"
              title={t('error.grants')}
              subtitle={grantError}
              lowContrast
              hideCloseButton
            />
          )}
          {!grantLoading && (
            <>
              <section className="users-page__grants-section">
                <h3>{t('grants.roles')}</h3>
                <p className="users-page__grants-hint">{t('grants.rolesHint')}</p>
                {grantRoles.map((role) => {
                  const entry = grantForm.roles[role.code] ?? { checked: false, expiresAt: '' }
                  const expired = entry.checked && entry.expiresAt && new Date(entry.expiresAt) < startOfToday()
                  return (
                    <div key={role.id} className="users-page__grant-row">
                      <Checkbox
                        id={`grant-role-${role.id}`}
                        labelText={role.name}
                        checked={entry.checked}
                        onChange={(_, { checked }) =>
                          setGrantForm({
                            ...grantForm,
                            roles: { ...grantForm.roles, [role.code]: { ...entry, checked } },
                          })
                        }
                      />
                      {role.isSystem && <Tag type="purple" size="sm">{t('grants.builtin')}</Tag>}
                      {entry.checked && (
                        <DatePicker
                          datePickerType="single"
                          dateFormat="Y-m-d"
                          locale={datePickerLocale()}
                          value={entry.expiresAt}
                          onChange={(_dates, dateStr) =>
                            setGrantForm({
                              ...grantForm,
                              roles: {
                                ...grantForm.roles,
                                [role.code]: { ...entry, expiresAt: dateStr },
                              },
                            })
                          }
                        >
                          <DatePickerInput
                            id={`grant-role-${role.id}-expiry`}
                            placeholder={t('grants.expiryPlaceholder')}
                            labelText={t('grants.expiryLabel')}
                            size="sm"
                          />
                        </DatePicker>
                      )}
                      {expired && <Tag type="red" size="sm">{t('grants.expired')}</Tag>}
                    </div>
                  )
                })}
              </section>
              <section className="users-page__grants-section">
                <h3>{t('grants.permissions')}</h3>
                <p className="users-page__grants-hint">{t('grants.permissionsHint')}</p>
                {catalogGroups.map((group) => (
                  <CheckboxGroup
                    key={group.name}
                    legendText={group.name}
                    className="users-page__perm-group"
                  >
                    {group.items.map((perm) => (
                      <Checkbox
                        key={perm.code}
                        id={`grant-perm-${perm.code}`}
                        labelText={t('grants.permOption', { name: perm.name, code: perm.code })}
                        checked={Boolean(grantForm.permissions[perm.code])}
                        onChange={(_, { checked }) =>
                          setGrantForm({
                            ...grantForm,
                            permissions: { ...grantForm.permissions, [perm.code]: checked },
                          })
                        }
                      />
                    ))}
                  </CheckboxGroup>
                ))}
              </section>
              <section className="users-page__grants-section">
                <h3>{t('grants.groups')}</h3>
                <p className="users-page__grants-hint">{t('grants.groupsHint')}</p>
                {grantForm.groups.length === 0 ? (
                  <p className="users-page__grants-empty">{t('grants.groupsEmpty')}</p>
                ) : (
                  <div className="users-page__tags">
                    {grantForm.groups.map((g) => (
                      <Tag key={g.id} type="cool-gray" size="sm">
                        {g.name}
                      </Tag>
                    ))}
                  </div>
                )}
              </section>
            </>
          )}
        </div>
      </Modal>

      {/* Delete */}
      <Modal
        danger
        open={Boolean(deleteTarget)}
        modalHeading={t('modal.delete')}
        primaryButtonText={t('modal.deleteSubmit')}
        secondaryButtonText={t('action.cancel', { ns: 'common' })}
        onRequestClose={() => setDeleteTarget(null)}
        onRequestSubmit={handleDelete}
        primaryButtonDisabled={deleting}
      >
        <p className="users-page__confirm-text">
          {t('deleteConfirm', { displayName: deleteTarget?.displayName, username: deleteTarget?.username })}
        </p>
        {deleteError && (
          <InlineNotification
            kind="error"
            title={t('error.delete')}
            subtitle={deleteError}
            lowContrast
            hideCloseButton
          />
        )}
      </Modal>
    </Grid>
  )
}

// toDateInput renders a nullable expiry as a Y-M-D string for the picker.
function toDateInput(value: string | null | undefined): string {
  if (!value) return ''
  const d = new Date(value)
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`
}

// localMidnightUTC converts a Y-M-D picker value to a UTC RFC3339 instant of
// local midnight, matching the backend's server-side comparison.
function localMidnightUTC(value: string): string {
  return new Date(`${value}T00:00:00`).toISOString()
}

function startOfToday(): Date {
  const now = new Date()
  return new Date(now.getFullYear(), now.getMonth(), now.getDate())
}

// groupCatalog buckets the permission catalog by categoryName, preserving
// the catalog's sorted order within each bucket.
function groupCatalog(catalog: CatalogPermission[]): Array<{ name: string; items: CatalogPermission[] }> {
  const groups = new Map<string, CatalogPermission[]>()
  for (const perm of catalog) {
    const bucket = groups.get(perm.categoryName)
    if (bucket) {
      bucket.push(perm)
    } else {
      groups.set(perm.categoryName, [perm])
    }
  }
  return Array.from(groups, ([name, items]) => ({ name, items }))
}
