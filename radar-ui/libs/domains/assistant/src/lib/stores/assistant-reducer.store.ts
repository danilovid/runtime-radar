import { createReducer, on } from '@ngrx/store';

import {
    ADD_ASSISTANT_CONVERSATION_DOC_ACTION,
    ADD_ASSISTANT_MESSAGE_DOC_ACTION,
    ADD_ASSISTANT_SECRET_DOC_ACTION,
    APPEND_ASSISTANT_DELTA_DOC_ACTION,
    DELETE_ASSISTANT_CONVERSATION_DOC_ACTION,
    FINISH_ASSISTANT_MESSAGE_DOC_ACTION,
    SET_ASSISTANT_ACTION_DOC_ACTION,
    SET_ASSISTANT_ACTION_STATE_DOC_ACTION,
    SET_ASSISTANT_ACTIVE_CONVERSATION_DOC_ACTION,
    SET_ASSISTANT_ATTACHMENTS_DOC_ACTION,
    SET_ASSISTANT_INTEGRATION_DOC_ACTION,
    SET_ASSISTANT_OPEN_DOC_ACTION,
    SET_ASSISTANT_VIEW_DOC_ACTION,
    UPDATE_ASSISTANT_TOOL_DOC_ACTION
} from './assistant-action.store';
import {
    AssistantActionState,
    AssistantConversation,
    AssistantMessage,
    AssistantState,
    AssistantStopReason,
    AssistantToolPhase,
    AssistantView
} from '../interfaces';

export const assistantInitialState: AssistantState = {
    isOpen: false,
    view: AssistantView.HOME,
    integrationId: '',
    conversations: [],
    activeConversationId: '',
    isStreaming: false,
    pendingAttachments: []
};

/** Applies an update to the conversation the widget currently shows. */
const updateActive = (
    state: AssistantState,
    update: (conversation: AssistantConversation) => AssistantConversation
): AssistantState => ({
    ...state,
    conversations: state.conversations.map((conversation) =>
        conversation.id === state.activeConversationId ? update(conversation) : conversation
    )
});

/**
 * Every update to the answer being streamed lands on the last message, which is
 * the pending one the effect added when the question was sent.
 */
const updateLastMessage = (
    state: AssistantState,
    update: (message: AssistantMessage) => AssistantMessage
): AssistantState =>
    updateActive(state, (conversation) => {
        if (!conversation.messages.length) {
            return conversation;
        }

        const messages = [...conversation.messages];
        messages[messages.length - 1] = update(messages[messages.length - 1]);

        return { ...conversation, messages };
    });

export const assistantReducer = createReducer(
    assistantInitialState,
    on(SET_ASSISTANT_OPEN_DOC_ACTION, (state, { isOpen }) => ({ ...state, isOpen })),
    on(SET_ASSISTANT_VIEW_DOC_ACTION, (state, { view }) => ({ ...state, view })),
    on(SET_ASSISTANT_INTEGRATION_DOC_ACTION, (state, { integrationId }) => ({ ...state, integrationId })),
    on(ADD_ASSISTANT_CONVERSATION_DOC_ACTION, (state, { conversation }) => ({
        ...state,
        conversations: [conversation, ...state.conversations],
        activeConversationId: conversation.id,
        view: AssistantView.CHAT
    })),
    on(SET_ASSISTANT_ACTIVE_CONVERSATION_DOC_ACTION, (state, { conversationId }) => ({
        ...state,
        activeConversationId: conversationId,
        view: AssistantView.CHAT
    })),
    on(DELETE_ASSISTANT_CONVERSATION_DOC_ACTION, (state, { conversationId }) => {
        const conversations = state.conversations.filter((conversation) => conversation.id !== conversationId);
        const isActive = state.activeConversationId === conversationId;

        return {
            ...state,
            conversations,
            activeConversationId: isActive ? '' : state.activeConversationId,
            view: isActive ? AssistantView.HOME : state.view,
            isStreaming: isActive ? false : state.isStreaming
        };
    }),
    on(ADD_ASSISTANT_MESSAGE_DOC_ACTION, (state, { message }) => ({
        ...updateActive(state, (conversation) => ({
            ...conversation,
            // The first question names the conversation, the way the chats page
            // lists it.
            title: conversation.title || message.content.slice(0, 60),
            messages: [...conversation.messages, message]
        })),
        isStreaming: message.isPending || state.isStreaming
    })),
    on(APPEND_ASSISTANT_DELTA_DOC_ACTION, (state, { delta }) =>
        updateLastMessage(state, (message) => ({ ...message, content: message.content + delta }))
    ),
    on(UPDATE_ASSISTANT_TOOL_DOC_ACTION, (state, { activity }) =>
        updateLastMessage(state, (message) => {
            const isRunning = activity.phase === AssistantToolPhase.STARTED;
            const tools = [...message.tools];
            // A finished tool updates the last still-running entry with that
            // name, so that the same tool called twice shows up twice.
            const index = tools.map((tool) => tool.name === activity.name && tool.isRunning).lastIndexOf(true);

            if (isRunning || index < 0) {
                tools.push({ name: activity.name, isRunning, error: activity.error });
            } else {
                tools[index] = { name: activity.name, isRunning: false, error: activity.error };
            }

            return { ...message, tools };
        })
    ),
    // An answer that stopped to ask for approval is not an error: the action
    // card it carries is the thing the user is meant to act on.
    on(FINISH_ASSISTANT_MESSAGE_DOC_ACTION, (state, { stopReason, error }) => ({
        ...updateLastMessage(state, (message) => ({
            ...message,
            isPending: false,
            stopReason: stopReason ?? (error ? AssistantStopReason.ERROR : message.stopReason),
            error: error ?? message.error,
            // A tool left running when the answer ended never reported back.
            tools: message.tools.map((tool) => ({ ...tool, isRunning: false }))
        })),
        isStreaming: false
    })),
    on(SET_ASSISTANT_ATTACHMENTS_DOC_ACTION, (state, { attachments }) => ({
        ...state,
        pendingAttachments: attachments
    })),
    on(SET_ASSISTANT_ACTION_DOC_ACTION, (state, { action }) =>
        updateLastMessage(state, (message) => ({ ...message, action }))
    ),
    on(SET_ASSISTANT_ACTION_STATE_DOC_ACTION, (state, { messageId, state: actionState }) =>
        updateActive(state, (conversation) => ({
            ...conversation,
            messages: conversation.messages.map((message) =>
                message.id === messageId && message.action
                    ? { ...message, action: { ...message.action, state: actionState } }
                    : message
            )
        }))
    ),
    on(ADD_ASSISTANT_SECRET_DOC_ACTION, (state, { secret }) =>
        updateLastMessage(state, (message) => ({ ...message, secrets: [...message.secrets, secret] }))
    )
);
