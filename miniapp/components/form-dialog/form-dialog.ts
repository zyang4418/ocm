/**
 * Centered form dialog used by every create/edit modal. Unlike t-dialog
 * (whose content area fights input focus/scroll), this is a plain overlay
 * with a scrollable body slot. The mask does NOT close on tap — forms hold
 * user input and accidental dismissal would lose it.
 */
Component({
  properties: {
    visible: { type: Boolean, value: false },
    title: { type: String, value: '' },
    confirmText: { type: String, value: '确定' },
    cancelText: { type: String, value: '取消' },
    loading: { type: Boolean, value: false },
    /** Disable the confirm button (client-side validation). */
    disabled: { type: Boolean, value: false },
    /** Inline error line above the footer. */
    error: { type: String, value: '' },
    /** Hide the footer (detail/preview dialogs manage their own actions). */
    hideFooter: { type: Boolean, value: false },
    /** Hide only the confirm button (read-only detail dialogs). */
    hideConfirm: { type: Boolean, value: false }
  },
  methods: {
    onCancel() {
      if (this.data.loading) return
      this.triggerEvent('cancel')
    },
    onConfirm() {
      if (this.data.loading || this.data.disabled) return
      this.triggerEvent('confirm')
    }
  }
})
