import {
  Header,
  HeaderContainer,
  HeaderGlobalAction,
  HeaderGlobalBar,
  HeaderMenuButton,
  HeaderName,
  HeaderSideNavItems,
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
  Logout,
  Notification,
  Settings,
  UserAvatar,
  UserMultiple,
} from '@carbon/icons-react'
import { useLocation, useNavigate } from 'react-router-dom'
import { useAuth } from '../auth/AuthContext.jsx'

// AppShell renders the Carbon UI Shell frame (header + side navigation +
// content area) shared by all authenticated pages. It persists across route
// changes via a React Router layout route so the header never remounts.
export default function AppShell({ children }) {
  const { user, logout } = useAuth()
  const navigate = useNavigate()
  const location = useLocation()

  const isActive = (path) => location.pathname === path
  const inUsers = location.pathname.startsWith('/users')
  const inClassrooms = location.pathname.startsWith('/classrooms')

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
            <Header aria-label="OCM 管理平台">
              <SkipToContent />
              <HeaderMenuButton
                aria-label={isSideNavExpanded ? '收起导航菜单' : '展开导航菜单'}
                onClick={onClickSideNavExpand}
                isActive={isSideNavExpanded}
                isCollapsible
              />
              <HeaderName href="/" prefix="" onClick={go('/')}>
                OCM 管理平台
              </HeaderName>
              <HeaderGlobalBar>
                <HeaderGlobalAction aria-label="通知" tooltipAlignment="end">
                  <Notification size={20} />
                </HeaderGlobalAction>
                <HeaderGlobalAction
                  aria-label={`当前用户：${user?.displayName ?? ''}`}
                  tooltipAlignment="end"
                >
                  <UserAvatar size={20} />
                </HeaderGlobalAction>
                <HeaderGlobalAction
                  aria-label="退出登录"
                  tooltipAlignment="end"
                  onClick={logout}
                >
                  <Logout size={20} />
                </HeaderGlobalAction>
              </HeaderGlobalBar>
              <SideNav
                aria-label="侧边导航"
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
                    概览
                  </SideNavLink>
                  <SideNavMenu
                    key={`cls-${inClassrooms}`}
                    renderIcon={Building}
                    title="教室管理"
                    defaultExpanded={inClassrooms}
                  >
                    <SideNavMenuItem
                      href="/classrooms"
                      isActive={isActive('/classrooms')}
                      onClick={go('/classrooms')}
                    >
                      教室列表
                    </SideNavMenuItem>
                  </SideNavMenu>
                  <SideNavMenu
                    key={`org-${inUsers}`}
                    renderIcon={UserMultiple}
                    title="组织与权限"
                    defaultExpanded={inUsers}
                  >
                    <SideNavMenuItem
                      href="/users"
                      isActive={isActive('/users')}
                      onClick={go('/users')}
                    >
                      用户管理
                    </SideNavMenuItem>
                    <SideNavMenuItem href="#">角色管理</SideNavMenuItem>
                  </SideNavMenu>
                  <SideNavMenu renderIcon={Settings} title="系统设置">
                    <SideNavMenuItem href="#">参数配置</SideNavMenuItem>
                    <SideNavMenuItem href="#">审计日志</SideNavMenuItem>
                  </SideNavMenu>
                </SideNavItems>
              </SideNav>
            </Header>
            <Theme theme="g10" className="app-shell__content">
              <Content id="main-content">{children}</Content>
            </Theme>
          </>
        )
      }}
    />
  )
}
