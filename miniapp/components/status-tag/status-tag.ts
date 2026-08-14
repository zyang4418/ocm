/** Small dot + text status badge, replacing Carbon's colored Tag. */
Component({
  properties: {
    text: { type: String, value: '' },
    theme: { type: String, value: 'gray' } // blue | green | red | gray | orange | purple
  }
})
