import { createAction, props } from '@ngrx/store';

import { AssistantMessage, AssistantStopReason, AssistantToolActivityResponse } from '../interfaces';

export const OPEN_ASSISTANT_TODO_ACTION = createAction(
    '[Assistant] Open',
    props<{ eventId?: string; question?: string }>()
);

export const CLOSE_ASSISTANT_TODO_ACTION = createAction('[Assistant] Close');

export const CLEAR_ASSISTANT_CONVERSATION_TODO_ACTION = createAction('[Assistant] Clear Conversation');

export const SELECT_ASSISTANT_INTEGRATION_TODO_ACTION = createAction(
    '[Assistant] Select Integration',
    props<{ integrationId: string }>()
);

export const SEND_ASSISTANT_MESSAGE_TODO_ACTION = createAction(
    '[Assistant] Send Message',
    props<{ content: string }>()
);

export const SET_ASSISTANT_OPEN_DOC_ACTION = createAction(
    '[Assistant] (Doc) Set Open',
    props<{ isOpen: boolean; eventId?: string }>()
);

export const SET_ASSISTANT_INTEGRATION_DOC_ACTION = createAction(
    '[Assistant] (Doc) Set Integration',
    props<{ integrationId: string }>()
);

export const ADD_ASSISTANT_MESSAGE_DOC_ACTION = createAction(
    '[Assistant] (Doc) Add Message',
    props<{ message: AssistantMessage }>()
);

/** A piece of the answer, appended to the message being streamed. */
export const APPEND_ASSISTANT_DELTA_DOC_ACTION = createAction(
    '[Assistant] (Doc) Append Delta',
    props<{ delta: string }>()
);

export const UPDATE_ASSISTANT_TOOL_DOC_ACTION = createAction(
    '[Assistant] (Doc) Update Tool',
    props<{ activity: AssistantToolActivityResponse }>()
);

export const FINISH_ASSISTANT_MESSAGE_DOC_ACTION = createAction(
    '[Assistant] (Doc) Finish Message',
    props<{ stopReason?: AssistantStopReason; error?: string }>()
);

export const CLEAR_ASSISTANT_CONVERSATION_DOC_ACTION = createAction('[Assistant] (Doc) Clear Conversation');
