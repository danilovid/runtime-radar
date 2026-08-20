import {
    ADD_ASSISTANT_MESSAGE_DOC_ACTION,
    APPEND_ASSISTANT_DELTA_DOC_ACTION,
    CLEAR_ASSISTANT_CONVERSATION_DOC_ACTION,
    FINISH_ASSISTANT_MESSAGE_DOC_ACTION,
    UPDATE_ASSISTANT_TOOL_DOC_ACTION
} from './assistant-action.store';
import { AssistantMessage, AssistantRole, AssistantStopReason, AssistantToolPhase } from '../interfaces';
import { assistantInitialState, assistantReducer } from './assistant-reducer.store';

const answer = (): AssistantMessage => ({
    id: 'answer',
    role: AssistantRole.ASSISTANT,
    content: '',
    tools: [],
    isPending: true
});

describe('assistantReducer', () => {
    it('streams an answer that ran a tool', () => {
        // The shape of a real answer: the tool starts, finishes, and the text
        // arrives after it.
        let state = assistantReducer(assistantInitialState, ADD_ASSISTANT_MESSAGE_DOC_ACTION({ message: answer() }));

        expect(state.isStreaming).toBe(true);

        state = assistantReducer(
            state,
            UPDATE_ASSISTANT_TOOL_DOC_ACTION({ activity: { name: 'search_docs', phase: AssistantToolPhase.STARTED } })
        );

        expect(state.messages[0].tools).toEqual([{ name: 'search_docs', isRunning: true, error: undefined }]);

        state = assistantReducer(
            state,
            UPDATE_ASSISTANT_TOOL_DOC_ACTION({ activity: { name: 'search_docs', phase: AssistantToolPhase.FINISHED } })
        );

        // The finished tool updates the entry rather than adding a second one.
        expect(state.messages[0].tools).toEqual([{ name: 'search_docs', isRunning: false, error: undefined }]);

        state = assistantReducer(state, APPEND_ASSISTANT_DELTA_DOC_ACTION({ delta: 'Open ' }));
        state = assistantReducer(state, APPEND_ASSISTANT_DELTA_DOC_ACTION({ delta: '**Response rules**.' }));

        expect(state.messages[0].content).toBe('Open **Response rules**.');

        state = assistantReducer(
            state,
            FINISH_ASSISTANT_MESSAGE_DOC_ACTION({ stopReason: AssistantStopReason.END_TURN })
        );

        expect(state.isStreaming).toBe(false);
        expect(state.messages[0].isPending).toBe(false);
        expect(state.messages[0].stopReason).toBe(AssistantStopReason.END_TURN);
    });

    it('records the same tool twice when it is called twice', () => {
        let state = assistantReducer(assistantInitialState, ADD_ASSISTANT_MESSAGE_DOC_ACTION({ message: answer() }));

        for (const phase of [AssistantToolPhase.STARTED, AssistantToolPhase.FINISHED, AssistantToolPhase.STARTED]) {
            state = assistantReducer(
                state,
                UPDATE_ASSISTANT_TOOL_DOC_ACTION({ activity: { name: 'search_runtime_events', phase } })
            );
        }

        expect(state.messages[0].tools).toHaveLength(2);
        expect(state.messages[0].tools[1].isRunning).toBe(true);
    });

    it('reports a failed tool', () => {
        let state = assistantReducer(assistantInitialState, ADD_ASSISTANT_MESSAGE_DOC_ACTION({ message: answer() }));

        state = assistantReducer(
            state,
            UPDATE_ASSISTANT_TOOL_DOC_ACTION({
                activity: { name: 'get_runtime_event', phase: AssistantToolPhase.STARTED }
            })
        );
        state = assistantReducer(
            state,
            UPDATE_ASSISTANT_TOOL_DOC_ACTION({
                activity: { name: 'get_runtime_event', phase: AssistantToolPhase.FINISHED, error: 'permission denied' }
            })
        );

        expect(state.messages[0].tools[0]).toEqual({
            name: 'get_runtime_event',
            isRunning: false,
            error: 'permission denied'
        });
    });

    it('stops a tool left running when the answer failed', () => {
        let state = assistantReducer(assistantInitialState, ADD_ASSISTANT_MESSAGE_DOC_ACTION({ message: answer() }));

        state = assistantReducer(
            state,
            UPDATE_ASSISTANT_TOOL_DOC_ACTION({ activity: { name: 'search_docs', phase: AssistantToolPhase.STARTED } })
        );
        state = assistantReducer(state, FINISH_ASSISTANT_MESSAGE_DOC_ACTION({ error: 'model is unreachable' }));

        expect(state.messages[0].tools[0].isRunning).toBe(false);
        expect(state.messages[0].error).toBe('model is unreachable');
        expect(state.messages[0].stopReason).toBe(AssistantStopReason.ERROR);
        expect(state.isStreaming).toBe(false);
    });

    it('clears the conversation and the event it was attached to', () => {
        let state = assistantReducer(
            { ...assistantInitialState, eventId: 'event-1' },
            ADD_ASSISTANT_MESSAGE_DOC_ACTION({ message: answer() })
        );

        state = assistantReducer(state, CLEAR_ASSISTANT_CONVERSATION_DOC_ACTION());

        expect(state.messages).toEqual([]);
        expect(state.eventId).toBe('');
        expect(state.isStreaming).toBe(false);
    });
});
