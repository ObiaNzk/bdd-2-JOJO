erDiagram
  DEPORTE ||--o{ DISCIPLINA : "agrupa"
  DISCIPLINA ||--o{ EVENTO : "se compite en"
  JUEGO_OLIMPICO ||--o{ EVENTO : "incluye"
  JUEGO_OLIMPICO ||--o{ JUEGO_OLIM_PAIS : "tiene participantes"
  PAIS ||--o{ JUEGO_OLIM_PAIS : "participa en"
  PAIS ||--o{ ATLETA : "nacionalidad"
  JUEGO_OLIM_PAIS ||--o{ EQUIPO : "inscribe"
  EVENTO ||--o{ EQUIPO : "compite en"
  EVENTO ||--|| CALENDARIO : "se realiza en"
  EVENTO ||--o| RESULTADOS : "tiene"
  EVENTO ||--o| ESTADISTICAS : "registra"
  EQUIPO ||--o{ EQUIPO_ATLETA : "compuesto por"
  ATLETA ||--o{ EQUIPO_ATLETA : "integra"
  EQUIPO ||--o| MEDALLERO : "obtiene"

  DEPORTE {
    int id_deporte PK
    string nombre
  }
  DISCIPLINA {
    int id_disciplina PK
    int id_deporte FK
    string nombre
  }
  JUEGO_OLIMPICO {
    int id_juego PK
    int anio
    string ciudad
    string tipo
  }
  PAIS {
    int id_pais PK
    string nombre
    string codigo_iso
  }
  JUEGO_OLIM_PAIS {
    int id_jo_pais PK
    int id_juego FK
    int id_pais FK
  }
  CALENDARIO {
    int id_calendario PK
    datetime fecha_hora
    string sede
  }
  EVENTO {
    int id_evento PK
    int id_juego FK
    int id_disciplina FK
    int id_calendario FK
    string nombre
    string genero
  }
  EQUIPO {
    int id_equipo PK
    int id_jo_pais FK
    int id_evento FK
  }
  EQUIPO_ATLETA {
    int id_equipo PK "FK"
    int id_atleta PK "FK"
  }
  ATLETA {
    int id_atleta PK
    int id_pais FK
    string nombre
    date fecha_nac
    string genero
  }
  RESULTADOS {
    int id_resultado PK
    int id_evento FK
    int id_equipo FK
    int posicion
    string marca
  }
  ESTADISTICAS {
    int id_estadistica PK
    int id_evento FK
    int id_equipo FK
    string metrica
    string valor
  }
  MEDALLERO {
    int id_medalla PK
    int id_equipo FK
    string tipo
  }