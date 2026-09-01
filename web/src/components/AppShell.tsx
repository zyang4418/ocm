import { lazy, Suspense, type MouseEvent, type ReactNode } from 'react'
import {
  Header,
  HeaderContainer,
  HeaderGlobalAction,
  HeaderGlobalBar,
  HeaderMenuButton,
  HeaderName,
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
  Contrast,
  Logout,
  Notification,
  Translate,
  UserAvatar,
} from '@carbon/icons-react'
import { useLocation, useNavigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { useAuth } from '../auth/AuthContext'
import useLanguage from '../i18n/useLanguage'
import { THEME_PREFERENCES, useTheme } from '../theme/ThemeContext'
import { navigation, type NavEntry, type NavItem, type PermissionGate } from '../config/navigation'

// The AI assistant chat widget is a large dependency chain (lit + Carbon web
// components), so it is only fetched when a user with ai:chat permission is
// logged in.
const AiChat = lazy(() => import('../ai/AiChat'))

/** True when the subject holds at least one of the gated permissions. */
function passesGate(can: (perm: string) => boolean, gate?: PermissionGate): boolean {
  if (!gate) return true
  return Array.isArray(gate) ? gate.some((perm) => can(perm)) : can(gate)
}

/** i18n label with a literal-label fallback for downstream entries. */
function navLabel(t: (key: string) => string, entry: { i18nKey?: string; label?: string; path?: string }): string {
  if (entry.i18nKey) return t(entry.i18nKey)
  return entry.label ?? entry.path ?? ''
}

// AppShell renders the Carbon UI Shell frame (header + side navigation +
// content area) shared by all authenticated pages. It persists across route
// changes via a React Router layout route so the header never remounts.
// The menu itself is data-driven — see src/config/navigation.ts.
export default function AppShell({ children }: { children: ReactNode }) {
  const { t } = useTranslation()
  const { language, setLanguage, languages } = useLanguage()
  const { user, logout, can } = useAuth()
  const { theme, setPreference } = useTheme()
  const navigate = useNavigate()
  const location = useLocation()

  const isActive = (path: string) => location.pathname === path

  const renderEntry = (
    entry: NavEntry,
    go: (path: string) => (e: MouseEvent) => void,
  ) => {
    if (!passesGate(can, entry.permission)) return null
    if (entry.kind === 'link') {
      return (
        <SideNavLink
          key={`link-${entry.path}`}
          renderIcon={entry.icon}
          href={entry.path}
          isActive={isActive(entry.path)}
          onClick={go(entry.path)}
        >
          {navLabel(t, entry)}
        </SideNavLink>
      )
    }
    const items = entry.items.filter((item) => passesGate(can, item.permission))
    if (items.length === 0) return null
    // SideNavMenu keeps its own open state after mount; the expanded flag in
    // the key forces a remount so the group auto-expands when the user
    // navigates deep into it from a bookmark (mirrors the previous
    // inClassrooms/inCourses/inOrg/inSettings booleans).
    const expanded = items.some((item) => location.pathname.startsWith(item.path))
    const key = `${entry.i18nKey ?? entry.label ?? items[0]?.path}-${expanded}`
    return (
      <SideNavMenu
        key={key}
        renderIcon={entry.icon}
        title={navLabel(t, entry)}
        defaultExpanded={expanded}
      >
        {items.map((item: NavItem) => (
          <SideNavMenuItem
            key={item.path}
            href={item.path}
            isActive={isActive(item.path)}
            onClick={go(item.path)}
          >
            {navLabel(t, item)}
          </SideNavMenuItem>
        ))}
      </SideNavMenu>
    )
  }

  return (
    <HeaderContainer
      render={({ isSideNavExpanded, onClickSideNavExpand }) => {
        const go = (path: string) => (e: MouseEvent) => {
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
                <OverflowMenu
                  renderIcon={Contrast}
                  aria-label={t('aria.themeSwitcher')}
                  iconDescription={t('aria.themeSwitcher')}
                  align="bottom-end"
                  flipped
                  className="app-shell__theme-switcher"
                >
                  {THEME_PREFERENCES.map((pref) => (
                    <OverflowMenuItem
                      key={pref}
                      itemText={t(`theme.${pref}`)}
                      isDelete={false}
                      onClick={() => setPreference(pref)}
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
                  {navigation.map((entry) => renderEntry(entry, go))}
                </SideNavItems>
              </SideNav>
            </Header>
            {/* The theme class is also set on <html> (ThemeContext) so the
                header, side nav and login page follow it; this Theme wrapper
                keeps the content area's layer semantics (cds--layer-one). */}
            <Theme theme={theme} className="app-shell__content">
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
