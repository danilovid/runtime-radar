import { NgModule } from '@angular/core';
import { RouterModule, Routes } from '@angular/router';

import { AssistantFeatureChatsContainer } from './containers/chats/assistant-chats.container';

const routes: Routes = [
    {
        path: '',
        component: AssistantFeatureChatsContainer
    }
];

@NgModule({
    imports: [RouterModule.forChild(routes)],
    exports: [RouterModule]
})
export class AssistantFeatureRoutingModule {}
