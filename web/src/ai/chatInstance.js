// Shared holder for the Carbon AI Chat instance. The chat widget itself is
// lazy-loaded (React.lazy), so code outside it (e.g. the dashboard quick link)
// uses these helpers to request the window instead of importing the widget.
// 'mainWindow' is ChatInstance.changeView(ViewType.MAIN_WINDOW).
export const aiChatState = {
  instance: null,
  openRequested: false,
}

export function openAiChat() {
  aiChatState.openRequested = true
  aiChatState.instance?.changeView('mainWindow')
}
