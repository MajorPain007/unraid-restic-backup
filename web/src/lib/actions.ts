export interface Actions {
  editJob: (id: string | null) => void;
  editRepo: (id: string | null) => void;
  previewJob: (jobId: string) => void;
  sizeHistory: (which: { job?: string; repo?: string }) => void;
  showLog: (opId: string) => void;
}

function missing(name: string) {
  return () => {
    throw new Error(name + ' is not available yet');
  };
}

export const actions: Actions = {
  editJob: missing('editJob'),
  editRepo: missing('editRepo'),
  previewJob: missing('previewJob'),
  sizeHistory: missing('sizeHistory'),
  showLog: missing('showLog')
};

export function provide(part: Partial<Actions>): void {
  Object.assign(actions, part);
}
