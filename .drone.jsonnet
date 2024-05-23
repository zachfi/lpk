local image = 'zachfi/shell:latest';

local pipeline(name) = {
  kind: 'pipeline',
  name: name,
  steps: [],
  // depends_on: [],
  // volumes: [],
};

[
  pipeline('build') {
    steps: [
      {
        name: 'build',
        image: image,
        commands: ['make build'],
      },
    ],
  },
]
