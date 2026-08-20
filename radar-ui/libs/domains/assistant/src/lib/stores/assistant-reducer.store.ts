import { createReducer, on } from '@ngrx/store';

import {
    ADD_ASSISTANT_MESSAGE_DOC_ACTION,
    APPEND_ASSISTANT_DELTA_DOC_ACTION,
    CLEAR_ASSISTANT_CONVERSATION_DOC_ACTION,
    FINISH_ASSISTANT_MESSAGE_DOC_ACTION,
    SET_ASSISTANT_INTEGRATION_DOC_ACTION,
    SET_ASSISTANT_OPEN_DOC_ACTION,
    UPDATE_ASSISTANT_TOOL_DOC_ACTION
} from './assistant-action.store';
import { AssistantState, AssistantStopReason, AssistantToolPhase } from '../interfaces';

export const assistantInitialState: AssistantState = {
    isOpen: false,
    integrationId: '',
    messages: [],
    isStreaming: false,
    eventId: ''
};

/**
 * Every update to the answer being streamed lands on the last message, which is
 * the pending one the effect added when the question was sent.
 */
const updateLastMessage = (
    state: AssistantState,
    update: (message: AssistantState['messages'][number]) => AssistantState['messages'][number]
): AssistantState => {
    if (!state.messages.length) {
        return state;
    }

    const messages = [...state.messages];
    messages[messages.length - 1] = update(messages[messages.length - 1]);

    return { ...state, messages };
};

export const assistantReducer = createReducer(
    assistantInitialState,
    on(SET_ASSISTANT_OPEN_DOC_ACTION, (state, { isOpen, eventId }) => ({
        ...state,
        isOpen,
        eventId: eventId ?? state.eventId
    })),
    on(SET_ASSISTANT_INTEGRATION_DOC_ACTION, (state, { integrationId }) => ({ ...state, integrationId })),
    on(ADD_ASSISTANT_MESSAGE_DOC_ACTION, (state, { message }) => ({
        ...state,
        messages: [...state.messages, message],
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
    on(CLEAR_ASSISTANT_CONVERSATION_DOC_ACTION, (state) => ({
        ...state,
        messages: [],
        isStreaming: false,
        eventId: ''
    }))
);
