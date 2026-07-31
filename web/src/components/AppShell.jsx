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
  Dashboard,
  Logout,
  Notification,
  Settings,
  UserAvatar,
  UserMultiple,
} from '@carbon/icons-react'
import { useAuth } from '../auth/AuthContext.jsx'

// AppShell renders the Carbon UI Shell frame (header + side navigation +
// content area) shared by all authenticated pages.
export default function AppShell({ children }) {
  const { user, logout } = useAuth()

  return (
    <HeaderContainer
      render={({ isSideNavExpanded, onClickSideNavExpand }) => (
        <>
          <Header aria-label="OCM 管理平台">
            <SkipToContent />
            <HeaderMenuButton
              aria-label={isSideNavExpanded ? '收起导航菜单' : '展开导航菜单'}
              onClick={onClickSideNavExpand}
              isActive={isSideNavExpanded}
            />
            <HeaderName href="/" prefix="">
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
              <HeaderGlobalAction aria-label="退出登录" tooltipAlignment="end" onClick={logout}>
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
                <SideNavLink renderIcon={Dashboard} href="/" isActive>
                  概览
                </SideNavLink>
                <SideNavMenu renderIcon={UserMultiple} title="组织与权限">
                  <SideNavMenuItem href="#">用户管理</SideNavMenuItem>
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
      )}
    />
  )
}
