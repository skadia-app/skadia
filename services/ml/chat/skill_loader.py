"""Load skill modules from YAML files."""
from __future__ import annotations

from dataclasses import dataclass, field
from pathlib import Path

import yaml


SKILLS_DIR = Path(__file__).parent / "skills"


@dataclass
class Skill:
    name: str
    description: str
    schema_excerpt: str
    domain_knowledge: str
    tools: list[str]
    few_shot_examples: list[dict]

    def to_context_string(self) -> str:
        """Format skill as context block for system prompt injection."""
        lines = [
            f"## Kontext: {self.description}",
            "",
            "### Schema",
            self.schema_excerpt.strip(),
            "",
            "### Hinweise",
            self.domain_knowledge.strip(),
        ]
        return "\n".join(lines)

    def format_few_shots(self, count: int = 3) -> str:
        """Format few-shot examples for prompt injection."""
        examples = self.few_shot_examples[:count]
        lines = ["## Beispiele"]
        for ex in examples:
            lines.append(f'Frage: "{ex["q"]}"')
            lines.append(f"→ {ex['tool']}({ex.get('args', '')})")
            lines.append(f"→ {ex['answer']}")
            lines.append("")
        return "\n".join(lines)


def load_skills(skills_dir: Path | None = None) -> dict[str, Skill]:
    """Load all skill YAML files from directory."""
    directory = skills_dir or SKILLS_DIR
    skills = {}

    for yaml_file in sorted(directory.glob("*.yaml")):
        with open(yaml_file) as f:
            data = yaml.safe_load(f)

        skill = Skill(
            name=data["name"],
            description=data["description"],
            schema_excerpt=data.get("schema_excerpt", ""),
            domain_knowledge=data.get("domain_knowledge", ""),
            tools=data.get("tools", []),
            few_shot_examples=data.get("few_shot_examples", []),
        )
        skills[skill.name] = skill

    return skills
