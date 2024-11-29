local pipeline(name) = {
  kind: 'pipeline',
  name: name,
  steps: [],
  // depends_on: [],
  // volumes: [],
};

local step(name) = {
  name: name,
  image: 'zachfi/build-image',
  pull: 'always',
  commands: [],
};


local make(target) = step(target) {
  commands: ['make %s' % target],
};

[
  pipeline('build') {
    steps: [
      make('build'),
    ],
  },
]
