package main

var PenduWords = []string{
	"DEVELOPPEUR", "WHATSAPP", "PYTHON", "GOLANG", "ALGORITHME",
	"ORDINATEUR", "INTERNET", "INTELLIGENCE", "ARTIFICIELLE", "ROBOT",
	"TRADING", "FINANCE", "BOURSE", "STRATEGIE", "RENTABILITE",
	"MESSAGERIE", "COMMUNAUTE", "GROUPE", "POULGA", "BLOCKCHAIN",
	"CRYPTOGRAPHIE", "DATABASE", "FRONTEND", "BACKEND", "FULLSTACK",
	"MICROSERVICES", "KUBERNETES", "DOCKER", "LINUX", "SERVEUR",
	"TELEPHONE", "APPLICATION", "LOGICIEL", "PROGRAMMATION", "FONCTION",
	"VARIABLE", "CONSTANTE", "INTERFACE", "RESEAU", "PROTOCOLE",
	"SATELLITE", "ESPACE", "PLANETE", "GALAXIE", "UNIVERS",
	"HISTOIRE", "GEOGRAPHIE", "PHILOSOPHIE", "LITTERATURE", "MUSIQUE",
	"AVENTURE", "MYSTERE", "CHOCOLAT", "VOYAGE", "MONTAGNE",
}

type QuizQuestion struct {
	Question string   `json:"question"`
	Options  []string `json:"options"`
	Answer   int      `json:"answer"` // 0-based index
	XP       int      `json:"xp"`
}

var QuizQuestions = []QuizQuestion{
	{
		Question: "Quel est le langage de programmation principal du cerveau de Poulga ?",
		Options:  []string{"Python", "JavaScript", "Go (Golang)", "C++"},
		Answer:   2,
		XP:       15,
	},
	{
		Question: "Quelle base de données est utilisée pour stocker mes souvenirs ?",
		Options:  []string{"MySQL", "PostgreSQL", "MongoDB", "Redis"},
		Answer:   1,
		XP:       15,
	},
	{
		Question: "Dans le trading, que signifie l'acronyme 'ROI' ?",
		Options:  []string{"Risk On Investment", "Return On Investment", "Rate Of Interest", "Return On Income"},
		Answer:   1,
		XP:       20,
	},
	{
		Question: "Quel concept de trading permet de limiter les pertes ?",
		Options:  []string{"Take Profit", "Leverage", "Stop Loss", "Margin Call"},
		Answer:   2,
		XP:       20,
	},
	{
		Question: "Quel est le plus grand océan de la Terre ?",
		Options:  []string{"Atlantique", "Indien", "Arctique", "Pacifique"},
		Answer:   3,
		XP:       10,
	},
	{
		Question: "Qui a peint la Joconde ?",
		Options:  []string{"Michel-Ange", "Léonard de Vinci", "Picasso", "Van Gogh"},
		Answer:   1,
		XP:       10,
	},
	{
		Question: "Quelle est la capitale du Japon ?",
		Options:  []string{"Pékin", "Séoul", "Tokyo", "Bangkok"},
		Answer:   2,
		XP:       10,
	},
	{
		Question: "Quel est l'élément chimique dont le symbole est 'O' ?",
		Options:  []string{"Or", "Oxygène", "Osmium", "Ozone"},
		Answer:   1,
		XP:       10,
	},
	{
		Question: "En quelle année l'homme a-t-il marché sur la Lune pour la première fois ?",
		Options:  []string{"1965", "1969", "1972", "1975"},
		Answer:   1,
		XP:       15,
	},
	{
		Question: "Quel pays a remporté la Coupe du Monde de football en 2022 ?",
		Options:  []string{"France", "Brésil", "Allemagne", "Argentine"},
		Answer:   3,
		XP:       15,
	},
	{
		Question: "Quel est l'inventeur de l'ampoule électrique ?",
		Options:  []string{"Nikola Tesla", "Alexander Graham Bell", "Thomas Edison", "Albert Einstein"},
		Answer:   2,
		XP:       15,
	},
	{
		Question: "Combien y a-t-il de planètes dans notre système solaire ?",
		Options:  []string{"7", "8", "9", "10"},
		Answer:   1,
		XP:       10,
	},
	{
		Question: "Quel est le plus long fleuve du monde ?",
		Options:  []string{"Amazone", "Nil", "Yangtsé", "Mississippi"},
		Answer:   0,
		XP:       15,
	},
	{
		Question: "Quelle est la monnaie du Royaume-Uni ?",
		Options:  []string{"Euro", "Dollar", "Livre Sterling", "Yen"},
		Answer:   2,
		XP:       10,
	},
	{
		Question: "Qui a écrit 'Les Misérables' ?",
		Options:  []string{"Gustave Flaubert", "Victor Hugo", "Émile Zola", "Honoré de Balzac"},
		Answer:   1,
		XP:       15,
	},
}

