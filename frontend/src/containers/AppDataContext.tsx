import React from 'react';
import { AppData } from 'common/types';

export const AppDataContext = React.createContext<AppData>({
  pluginID: undefined,
});
