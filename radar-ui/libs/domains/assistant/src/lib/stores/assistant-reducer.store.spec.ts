import {
    ADD_ASSISTANT_CONVERSATION_DOC_ACTION,
    ADD_ASSISTANT_MESSAGE_DOC_ACTION,
    APPEND_ASSISTANT_DELTA_DOC_ACTION,
    DELETE_ASSISTANT_CONVERSATION_DOC_ACTION,
    FINISH_ASSISTANT_MESSAGE_DOC_ACTION,
    SET_ASSISTANT_ATTACHMENTS_DOC_ACTION,
    UPDATE_ASSISTANT_TOOL_DOC_ACTION
} from './assistant-action.store';
import {
    AssistantConversation,
    AssistantMessage,
    AssistantMode,
    AssistantRole,
    AssistantState,
    AssistantStopReason,
    AssistantToolPhase,
    AssistantView
} from '../interfaces';
import { assistantInitialState, assistantReducer } from './assistant-reducer.store';

const conversation = (): AssistantConversation => ({
    id: 'conversation',
    title: '',
    createdAt: '2026-08-20T00:00:00.000Z',
    updatedAt: '2026-08-20T00:00:00.000Z',
    messages: [],
    eventId: '',
    mode: AssistantMode.CHAT
});

const message = (role: AssistantRole, content = ''): AssistantMessage => ({
    id: `${role}-message`,
    role,
    content,
    tools: [],
    attachments: [],
    secrets: [],
    isPending: role === AssistantRole.ASSISTANT
});

/** A state with one open conversation, which is where every answer lands. */
const withConversation = (): AssistantState =>
    assistantReducer(assistantInitialState, ADD_ASSISTANT_CONVERSATION_DOC_ACTION({ conversation: conversation() }));

const messagesOf = (state: AssistantState) => state.conversations[0].messages;

describe('assistantReducer', () => {
    it('opens a conversation and shows it', () => {
        const state = withConversation();

        expect(state.conversations).toHaveLength(1);
        expect(state.activeConversationId).toBe('conversation');
        expect(state.view).toBe(AssistantView.CHAT);
    });

    it('names a conversation after its first question', () => {
        let state = withConversation();

        state = assistantReducer(
            state,
            ADD_ASSISTANT_MESSAGE_DOC_ACTION({ message: message(AssistantRole.USER, 'how do I create a rule?') })
        );

        expect(state.conversations[0].title).toBe('how do I create a rule?');
    });

    it('streams an answer that ran a tool', () => {
        let state = withConversation();

        state = assistantReducer(
            state,
            ADD_ASSISTANT_MESSAGE_DOC_ACTION({ message: message(AssistantRole.ASSISTANT) })
        );

        expect(state.isStreaming).toBe(true);

        state = assistantReducer(
            state,
            UPDATE_ASSISTANT_TOOL_DOC_ACTION({ activity: { name: 'search_docs', phase: AssistantToolPhase.STARTED } })
        );

        expect(messagesOf(state)[0].tools).toEqual([{ name: 'search_docs', isRunning: true, error: undefined }]);

        state = assistantReducer(
            state,
            UPDATE_ASSISTANT_TOOL_DOC_ACTION({ activity: { name: 'search_docs', phase: AssistantToolPhase.FINISHED } })
        );

        // The finished tool updates the entry rather than adding a second one.
        expect(messagesOf(state)[0].tools).toEqual([{ name: 'search_docs', isRunning: false, error: undefined }]);

        state = assistantReducer(state, APPEND_ASSISTANT_DELTA_DOC_ACTION({ delta: 'Open ' }));
        state = assistantReducer(state, APPEND_ASSISTANT_DELTA_DOC_ACTION({ delta: '**Response rules**.' }));

        expect(messagesOf(state)[0].content).toBe('Open **Response rules**.');

        state = assistantReducer(
            state,
            FINISH_ASSISTANT_MESSAGE_DOC_ACTION({ stopReason: AssistantStopReason.END_TURN })
        );

        expect(state.isStreaming).toBe(false);
        expect(messagesOf(state)[0].isPending).toBe(false);
        expect(messagesOf(state)[0].stopReason).toBe(AssistantStopReason.END_TURN);
    });

    it('records the same tool twice when it is called twice', () => {
        let state = assistantReducer(
            withConversation(),
            ADD_ASSISTANT_MESSAGE_DOC_ACTION({ message: message(AssistantRole.ASSISTANT) })
        );

        for (const phase of [AssistantToolPhase.STARTED, AssistantToolPhase.FINISHED, AssistantToolPhase.STARTED]) {
            state = assistantReducer(
                state,
                UPDATE_ASSISTANT_TOOL_DOC_ACTION({ activity: { name: 'search_runtime_events', phase } })
            );
        }

        expect(messagesOf(state)[0].tools).toHaveLength(2);
        expect(messagesOf(state)[0].tools[1].isRunning).toBe(true);
    });

    it('stops a tool left running when the answer failed', () => {
        let state = assistantReducer(
            withConversation(),
            ADD_ASSISTANT_MESSAGE_DOC_ACTION({ message: message(AssistantRole.ASSISTANT) })
        );

        state = assistantReducer(
            state,
            UPDATE_ASSISTANT_TOOL_DOC_ACTION({ activity: { name: 'search_docs', phase: AssistantToolPhase.STARTED } })
        );
        state = assistantReducer(state, FINISH_ASSISTANT_MESSAGE_DOC_ACTION({ error: 'model is unreachable' }));

        expect(messagesOf(state)[0].tools[0].isRunning).toBe(false);
        expect(messagesOf(state)[0].error).toBe('model is unreachable');
        expect(messagesOf(state)[0].stopReason).toBe(AssistantStopReason.ERROR);
        expect(state.isStreaming).toBe(false);
    });

    it('deleting the open conversation returns to the home screen', () => {
        let state = withConversation();

        state = assistantReducer(state, DELETE_ASSISTANT_CONVERSATION_DOC_ACTION({ conversationId: 'conversation' }));

        expect(state.conversations).toEqual([]);
        expect(state.activeConversationId).toBe('');
        expect(state.view).toBe(AssistantView.HOME);
    });

    it('keeps picked attachments until they are sent', () => {
        const attachment = { name: 'deploy.yaml', size: 10, content: 'kind: Pod', isTruncated: false };

        let state = assistantReducer(
            assistantInitialState,
            SET_ASSISTANT_ATTACHMENTS_DOC_ACTION({ attachments: [attachment] })
        );

        expect(state.pendingAttachments).toEqual([attachment]);

        state = assistantReducer(state, SET_ASSISTANT_ATTACHMENTS_DOC_ACTION({ attachments: [] }));

        expect(state.pendingAttachments).toEqual([]);
    });
});
