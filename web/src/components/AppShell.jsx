import { lazy, Suspense } from 'react'
import {
  Header,
  HeaderContainer,
  HeaderGlobalAction,
  HeaderGlobalBar,
  HeaderMenuButton,
  HeaderName,
  HeaderSideNavItems,
  OverflowMenu,
  OverflowMenuItem,
  SideNav,
  SideNavItems,
  SideNavLink,
  SideNavMenu,
  SideNavMenuItem,
  SkipToContent,
  Content,
  Theme,
} from '@carbon/react'
import {
  Building,
  Dashboard,
  Education,
  Logout,
  Notification,
  Settings,
  Translate,
  UserAvatar,
  UserMultiple,
} from '@carbon/icons-react'
import { useLocation, useNavigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { useAuth } from '../auth/AuthContext.jsx'
import useLanguage from '../i18n/useLanguage.js'

// The AI assistant chat widget is a large dependency chain (lit + Carbon web
// components), so it is only fetched when a user with ai:chat permission is
// logged in.
const AiChat = lazy(() => import('../ai/AiChat.jsx'))

// AppShell renders the Carbon UI Shell frame (header + side navigation +
// content area) shared by all authenticated pages. It persists across route
// changes via a React Router layout route so the header never remounts.
export default function AppShell({ children }) {
  const { t } = useTranslation()
  const { language, setLanguage, languages } = useLanguage()
  const { user, logout, can } = useAuth()
  const navigate = useNavigate()
  const location = useLocation()

  const isActive = (path) => location.pathname === path
  const inOrg =
    location.pathname.startsWith('/users') ||
    location.pathname.startsWith('/admin-classes') ||
    location.pathname.startsWith('/teaching-classes') ||
    location.pathname.startsWith('/roles') ||
    location.pathname.startsWith('/groups')
  const inClassrooms =
    location.pathname.startsWith('/classrooms') ||
    location.pathname.startsWith('/bookings') ||
    location.pathname.startsWith('/repairs')
  const inCourses =
    location.pathname.startsWith('/courses') ||
    location.pathname.startsWith('/timetable') ||
    location.pathname.startsWith('/schedule-config') ||
    location.pathname.startsWith('/imports') ||
    location.pathname.startsWith('/attendance') ||
    location.pathname.startsWith('/observations')
  const inSettings =
    location.pathname.startsWith('/logs') ||
    location.pathname.startsWith('/settings')

  return (
    <HeaderContainer
      render={({ isSideNavExpanded, onClickSideNavExpand }) => {
        const go = (path) => (e) => {
          e.preventDefault()
          navigate(path)
          if (isSideNavExpanded) onClickSideNavExpand()
        }

        return (
          <>
            <Header aria-label={t('app.title')}>
              <SkipToContent />
              <HeaderMenuButton
                aria-label={isSideNavExpanded ? t('aria.navToggleClose') : t('aria.navToggleOpen')}
                onClick={onClickSideNavExpand}
                isActive={isSideNavExpanded}
                isCollapsible
              />
              <HeaderName href="/" prefix="" onClick={go('/')}>
                {t('app.title')}
              </HeaderName>
              <HeaderGlobalBar>
                <HeaderGlobalAction aria-label={t('aria.notification')} tooltipAlignment="end">
                  <Notification size={20} />
                </HeaderGlobalAction>
                <OverflowMenu
                  renderIcon={Translate}
                  aria-label={t('aria.languageSwitcher')}
                  iconDescription={t('aria.languageSwitcher')}
                  align="bottom-end"
                  flipped
                  className="app-shell__lang-switcher"
                >
                  {languages.map((lng) => (
                    <OverflowMenuItem
                      key={lng}
                      itemText={t(`language.${lng}`)}
                      isDelete={false}
                      onClick={() => setLanguage(lng)}
                    />
                  ))}
                </OverflowMenu>
                <HeaderGlobalAction
                  aria-label={t('aria.currentUser', { name: user?.displayName ?? '' })}
                  tooltipAlignment="end"
                >
                  <UserAvatar size={20} />
                </HeaderGlobalAction>
                <HeaderGlobalAction
                  aria-label={t('aria.logout')}
                  tooltipAlignment="end"
                  onClick={logout}
                >
                  <Logout size={20} />
                </HeaderGlobalAction>
              </HeaderGlobalBar>
              <SideNav
                aria-label={t('aria.sideNav')}
                expanded={isSideNavExpanded}
                isPersistent={false}
                onOverlayClick={onClickSideNavExpand}
              >
                <SideNavItems>
                  <SideNavLink
                    renderIcon={Dashboard}
                    href="/"
                    isActive={isActive('/')}
                    onClick={go('/')}
                  >
                    {t('nav.overview')}
                  </SideNavLink>
                  <SideNavMenu
                    key={`cls-${inClassrooms}`}
                    renderIcon={Building}
                    title={t('nav.classroomManagement')}
                    defaultExpanded={inClassrooms}
                  >
                    <SideNavMenuItem
                      href="/classrooms"
                      isActive={isActive('/classrooms')}
                      onClick={go('/classrooms')}
                    >
                      {t('nav.classroomList')}
                    </SideNavMenuItem>
                    <SideNavMenuItem
                      href="/bookings"
                      isActive={isActive('/bookings')}
                      onClick={go('/bookings')}
                    >
                      {t('nav.classroomBooking')}
                    </SideNavMenuItem>
                    {(can('repair:create') || can('repair:assign')) && (
                      <SideNavMenuItem
                        href="/repairs"
                        isActive={isActive('/repairs')}
                        onClick={go('/repairs')}
                      >
                        {t('nav.classroomRepair')}
                      </SideNavMenuItem>
                    )}
                  </SideNavMenu>
                  <SideNavMenu
                    key={`course-${inCourses}`}
                    renderIcon={Education}
                    title={t('nav.teachingManagement')}
                    defaultExpanded={inCourses}
                  >
                    <SideNavMenuItem
                      href="/courses"
                      isActive={isActive('/courses')}
                      onClick={go('/courses')}
                    >
                      {t('nav.courseManagement')}
                    </SideNavMenuItem>
                    <SideNavMenuItem
                      href="/timetable"
                      isActive={isActive('/timetable')}
                      onClick={go('/timetable')}
                    >
                      {t('nav.timetable')}
                    </SideNavMenuItem>
                    <SideNavMenuItem
                      href="/schedule-config"
                      isActive={isActive('/schedule-config')}
                      onClick={go('/schedule-config')}
                    >
                      {t('nav.scheduleConfig')}
                    </SideNavMenuItem>
                    {can('course:manage') && (
                      <SideNavMenuItem
                        href="/imports"
                        isActive={isActive('/imports')}
                        onClick={go('/imports')}
                      >
                        {t('nav.dataImport')}
                      </SideNavMenuItem>
                    )}
                    {can('course:manage') && (
                      <SideNavMenuItem
                        href="/imports/split"
                        isActive={isActive('/imports/split')}
                        onClick={go('/imports/split')}
                      >
                        {t('nav.jwcSplit')}
                      </SideNavMenuItem>
                    )}
                    {can('attendance:read') && (
                      <SideNavMenuItem
                        href="/attendance"
                        isActive={isActive('/attendance')}
                        onClick={go('/attendance')}
                      >
                        {t('nav.attendance')}
                      </SideNavMenuItem>
                    )}
                    {can('attendance:read') && (
                      <SideNavMenuItem
                        href="/attendance/report"
                        isActive={isActive('/attendance/report')}
                        onClick={go('/attendance/report')}
                      >
                        {t('nav.attendanceReport')}
                      </SideNavMenuItem>
                    )}
                    {can('observation:read') && (
                      <SideNavMenuItem
                        href="/observations"
                        isActive={isActive('/observations')}
                        onClick={go('/observations')}
                      >
                        {t('nav.observations')}
                      </SideNavMenuItem>
                    )}
                  </SideNavMenu>
                  <SideNavMenu
                    key={`org-${inOrg}`}
                    renderIcon={UserMultiple}
                    title={t('nav.orgManagement')}
                    defaultExpanded={inOrg}
                  >
                    {can('user:read') && (
                      <SideNavMenuItem
                        href="/users"
                        isActive={isActive('/users')}
                        onClick={go('/users')}
                      >
                        {t('nav.userManagement')}
                      </SideNavMenuItem>
                    )}
                    <SideNavMenuItem
                      href="/admin-classes"
                      isActive={isActive('/admin-classes')}
                      onClick={go('/admin-classes')}
                    >
                      {t('nav.adminClasses')}
                    </SideNavMenuItem>
                    <SideNavMenuItem
                      href="/teaching-classes"
                      isActive={isActive('/teaching-classes')}
                      onClick={go('/teaching-classes')}
                    >
                      {t('nav.teachingClasses')}
                    </SideNavMenuItem>
                    {can('role:manage') && (
                      <SideNavMenuItem
                        href="/roles"
                        isActive={isActive('/roles')}
                        onClick={go('/roles')}
                      >
                        {t('nav.roleManagement')}
                      </SideNavMenuItem>
                    )}
                    {can('group:manage') && (
                      <SideNavMenuItem
                        href="/groups"
                        isActive={isActive('/groups')}
                        onClick={go('/groups')}
                      >
                        {t('nav.groupManagement')}
                      </SideNavMenuItem>
                    )}
                  </SideNavMenu>
                  {can('log:read') && (
                    <SideNavMenu
                      key={`settings-${inSettings}`}
                      renderIcon={Settings}
                      title={t('nav.systemSettings')}
                      defaultExpanded={inSettings}
                    >
                      {can('*') && (
                        <SideNavMenuItem
                          href="/settings"
                          isActive={isActive('/settings')}
                          onClick={go('/settings')}
                        >
                          {t('nav.parameters')}
                        </SideNavMenuItem>
                      )}
                      <SideNavMenuItem
                        href="/logs"
                        isActive={isActive('/logs')}
                        onClick={go('/logs')}
                      >
                        {t('nav.auditLogs')}
                      </SideNavMenuItem>
                    </SideNavMenu>
                  )}
                </SideNavItems>
              </SideNav>
            </Header>
            <Theme theme="g10" className="app-shell__content">
              <Content id="main-content">{children}</Content>
            </Theme>
            {can('ai:chat') && (
              <Suspense fallback={null}>
                <AiChat />
              </Suspense>
            )}
          </>
        )
      }}
    />
  )
}
