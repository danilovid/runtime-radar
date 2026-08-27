/**
 * A file attached to a question is read in the browser and travels as text
 * inside the message, so the files of one message share a single budget rather
 * than each getting one of their own.
 *
 * The budget is smaller than maxMessageBytes in notifier/pkg/assistant, which
 * bounds the whole message: what is left over covers the question itself and
 * the delimiters around each file. The server rejects a message over its limit
 * outright instead of trimming it, so anything the widget sends has to fit —
 * that is what this number is for.
 */
export const ASSISTANT_MAX_ATTACHMENTS_BYTES = 12 * 1024;

export const ASSISTANT_MAX_ATTACHMENTS = 3;

/** Extensions worth reading as text. Anything else is refused with a hint. */
export const ASSISTANT_ATTACHMENT_ACCEPT = '.yaml,.yml,.json,.log,.txt,.conf,.env,.sh,.md';

/**
 * Follow-up questions offered under an answer. They are static on purpose:
 * generating them would cost another model round trip per answer.
 */
export const ASSISTANT_SUGGESTION_KEYS = [
    'Assistant.Widget.Suggestion.WhatHappened',
    'Assistant.Widget.Suggestion.HowToRespond',
    'Assistant.Widget.Suggestion.SimilarEvents'
];

/**
 * The cards on the home screen, mirroring "popular actions" in the design.
 * Each one starts a conversation with the question as its first message.
 */
export const ASSISTANT_QUICK_ACTIONS = [
    { titleKey: 'Assistant.Widget.QuickAction.Rule.Title', hintKey: 'Assistant.Widget.QuickAction.Rule.Hint' },
    { titleKey: 'Assistant.Widget.QuickAction.Threats.Title', hintKey: 'Assistant.Widget.QuickAction.Threats.Hint' },
    {
        titleKey: 'Assistant.Widget.QuickAction.Integration.Title',
        hintKey: 'Assistant.Widget.QuickAction.Integration.Hint'
    },
    { titleKey: 'Assistant.Widget.QuickAction.Detectors.Title', hintKey: 'Assistant.Widget.QuickAction.Detectors.Hint' }
];

/** Topics of the product walkthrough. Each opens a chat with that question. */
export const ASSISTANT_TOUR_TOPICS = [
    { titleKey: 'Assistant.Widget.Tour.Sources.Title', descKey: 'Assistant.Widget.Tour.Sources.Desc' },
    { titleKey: 'Assistant.Widget.Tour.Detectors.Title', descKey: 'Assistant.Widget.Tour.Detectors.Desc' },
    { titleKey: 'Assistant.Widget.Tour.Rules.Title', descKey: 'Assistant.Widget.Tour.Rules.Desc' },
    { titleKey: 'Assistant.Widget.Tour.Notifications.Title', descKey: 'Assistant.Widget.Tour.Notifications.Desc' }
];
