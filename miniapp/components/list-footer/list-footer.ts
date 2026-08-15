/** Pagination footer: 加载中… / 没有更多了 / custom text (e.g. 共 N 条). */
Component({
  properties: {
    loading: { type: Boolean, value: false },
    noMore: { type: Boolean, value: false },
    text: { type: String, value: '' }
  }
})
