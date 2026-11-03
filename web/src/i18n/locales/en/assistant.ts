// The assistant's window.
export const assistant = {
  assistant: {
    open: 'Open the assistant',
    close: 'Close the assistant',
    title: 'Assistant',
    regionLabel: 'Assistant',
    newConversation: 'New conversation',
    conversations: 'Conversations',
    backToConversation: 'Back to the conversation',
    search: 'Search conversations',
    noConversations: 'No conversations yet',
    noMatches: 'No conversation matches',
    listFailed: 'Could not read the conversations',
    threadFailed: 'Could not read this conversation',
    retry: 'Try again',
    conversationActions: 'Conversation actions',
    delete: 'Delete',
    deleteTitle: 'Delete conversation',
    deleteMessage:
      '“{title}” goes, with what was said in it and its model calls',
    deleteFailed: 'Could not delete the conversation',
    clear: 'Clear conversations',
    clearMessage:
      'Every conversation goes, with what was said in it, its model calls and anything waiting for your confirmation',
    clearFailed: 'Could not clear the conversations',
    message: 'Message to the assistant',
    placeholder: 'Ask about your trading, or this page',
    empty: 'Ask about what is on this page',
    noModel: 'No model is set up yet',
    addModel: 'Add a model',
    model: 'Model',
    send: 'Send',
    stop: 'Stop',
    stopped: 'Stopped',
    stoppedDetail: 'Anything already done stays done',
    confirm: 'Confirm',
    cancel: 'Cancel',
    waiting: 'Waiting for your confirmation',
    waitingCount: 'Waiting for your confirmation · {count}',
    runAll: 'Run all {count}',
    realMoney: 'Real money',
    testnet: 'Testnet',
    placeholderWaiting: 'Or write something else to cancel it',
    results: 'What came of it',
    done: 'Done',
    failed: 'Failed',
    action: {
      create: 'Create trader',
      start: 'Start trader',
      stop: 'Stop trader',
      delete: 'Delete trader',
      update: 'Update trader',
      close: 'Close position',
      delete_conversation: 'Delete conversation',
      delete_conversations: 'Delete conversations',
      delete_plan: 'Delete plan',
      delete_plans: 'Delete plans',
    },
    fillIn: 'Fill in',
    dismissProposal: 'Dismiss',
    filled: 'Filled in. Look it over and save it on the page',
    formGone: 'That form is no longer open',
    openForm: 'Open it again',
    noChange: 'The form already holds these',
    emptyValue: 'empty',
    formTrader: 'Trader settings',
    formConfig: 'Config',
    formRequest: 'Request',
    stale: 'That no longer waits for an answer, so nothing was done',
    busy: 'The assistant is still answering. Wait for it or stop it first',
    dismiss: 'Dismiss',
    unread: 'New reply',
    suggest: {
      dashboard: {
        attention: 'What needs my attention?',
        attentionPrompt: 'What needs my attention right now, and why?',
        today: 'How is today going?',
        todayPrompt: 'How is today going across my accounts and traders?',
      },
      forecast_signals: {
        call: 'What does this call rest on?',
        callPrompt:
          'What does the call on this coin rest on, and how far can it be trusted?',
        record: 'How has the strategy done lately?',
        recordPrompt:
          'How has the strategy in use done lately, against what chance would hit?',
      },
      forecast_strategies: {
        record: 'How is this strategy doing?',
        recordPrompt:
          'How is this strategy doing live, against chance and against its backtest?',
        weakest: 'Where is it weakest?',
        weakestPrompt:
          'Where is this strategy weakest, by interval, market and call?',
      },
      forecast_backtests: {
        read: 'What does this run say?',
        readPrompt: 'What does this backtest say, and what does it not say?',
        chance: 'Does it beat chance?',
        chancePrompt:
          'Does this backtest beat chance after fees, with enough separate moments to mean anything?',
      },
      forecast_settings: {
        collecting: 'What is being collected?',
        collectingPrompt: 'What is Forecast collecting, and how is it doing?',
      },
      futures: {
        compare: 'Compare these plans',
        comparePrompt:
          'Compare the plans I have here: what each risks, what it could make, and which you would rather take',
        risks: 'What could go wrong?',
        risksPrompt:
          'What could go wrong with the plan I have open, and what would I see first?',
      },
      traders: {
        doing: 'How is this trader doing?',
        doingPrompt:
          'How is this trader doing, and what stands out in its recent cycles?',
        quiet: 'Why is it not trading?',
        quietPrompt:
          'Why has this trader not been trading lately? Check whether it can, and what it decided',
      },
      traders_overview: {
        best: 'Which trader is doing best?',
        bestPrompt:
          'Which trader is doing best after costs, and how much of that is luck?',
        attention: 'Which needs attention?',
        attentionPrompt: 'Which of my traders needs attention, and why?',
      },
    },
  },
}
