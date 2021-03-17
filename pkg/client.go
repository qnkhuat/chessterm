package pkg

import (
	"bufio"
	"fmt"
	"github.com/gdamore/tcell/v2"
	"github.com/notnil/chess"
	"github.com/rivo/tview"
	"log"
	"net"
	"strings"
	"time"
)

type Client struct {
	Game          *chess.Game
	App           *tview.Application
	Board         *tview.Table
	GameLayout    *tview.Grid
	MenuLayout    *tview.Grid
	Conn          net.Conn
	In            chan MessageInterface
	Out           chan MessageInterface
	selecting     bool
	lastSelection chess.Square
	highlights    map[chess.Square]bool
	Role          PlayerRole
	optionBtn1    *tview.Button // Draw, Accept, Yes
	optionBtn2    *tview.Button // Resign, Reject, No
}

var (
	ChatTextView   *tview.TextView
	StatusTextView *tview.TextView
	MenuTextView   *tview.TextView
)

const (
	numrows             = 8
	numcols             = 8
	numOfSquaresInBoard = 8 * 8
	ConnQueueSize       = 10
	commandlist         = `
In the lazyness of building an UI, gochess comes with a list of commands to join a game:

> [green]ls[white]            : List all the games
> [green]join [red](code)[white]   : Join a game.  Live blank to join randomly 
> [green]create [gray](code)[white] : Create a game with code name
> [green]callme [red](name)[white] : To set your name
> [green]help[white]          : To display this list
> [green]about[white]         : About the developer of Gochess
> [green]exit[white]          : To exit`
)

func NewClient() *Client {
	app := tview.NewApplication()

	In := make(chan MessageInterface, ConnQueueSize)
	Out := make(chan MessageInterface, ConnQueueSize)
	highlights := make(map[chess.Square]bool)
	cl := &Client{
		App:        app,
		Game:       chess.NewGame(chess.UseNotation(chess.UCINotation{})),
		In:         In,
		Out:        Out,
		highlights: highlights,
	}
	cl.InitGUI()

	return cl
}

func (cl *Client) Disconnect() {
	cl.App.Stop()
	cl.Conn.Close()
	log.Println("Disconnected")
}

func (cl *Client) HandleAction(action Action) {
	switch action {

	// Resign
	case ActionResignPrompt:
		cl.optionBtn1.SetLabel(string(ActionResignYes))
		cl.optionBtn2.SetLabel(string(ActionResignNo))

	case ActionResignYes:
		cl.Out <- MessageGameAction{Action: action}

	case ActionResignNo:
		cl.optionBtn1.SetLabel(string(ActionDrawPrompt))
		cl.optionBtn2.SetLabel(string(ActionResignPrompt))

	// Draw
	case ActionDrawOffer:
		cl.optionBtn1.SetLabel(string(ActionDrawAccept))
		cl.optionBtn2.SetLabel(string(ActionDrawReject))

	case ActionDrawPrompt:
		cl.Out <- MessageGameAction{Action: ActionDrawOffer}
		StatusTextView.SetText("Draw offer sent!")

	case ActionDrawAccept:
		cl.Out <- MessageGameAction{Action: action}
		cl.optionBtn1.SetLabel(string(ActionDrawPrompt))
		cl.optionBtn2.SetLabel(string(ActionResignPrompt))

	case ActionDrawReject:
		cl.Out <- MessageGameAction{Action: action}
		cl.optionBtn1.SetLabel(string(ActionDrawPrompt))
		cl.optionBtn2.SetLabel(string(ActionResignPrompt))

	// New Game
	case ActionNewGameOffer:
		cl.optionBtn1.SetLabel(string(ActionNewGameAccept))
		cl.optionBtn2.SetLabel(string(ActionNewGameReject))

	case ActionNewGamePrompt:
		cl.Out <- MessageGameAction{Action: ActionNewGameOffer}
		StatusTextView.SetText("Invitation sent!")

	case ActionNewGameAccept:
		cl.Out <- MessageGameAction{Action: action}
		cl.optionBtn1.SetLabel(string(ActionDrawPrompt))
		cl.optionBtn2.SetLabel(string(ActionResignPrompt))

	case ActionNewGameReject:
		cl.Out <- MessageGameAction{Action: action}
		cl.optionBtn1.SetLabel(string(ActionNewGamePrompt))
		cl.optionBtn2.SetLabel(string(ActionExit))

	// Result
	case ActionWin, ActionLose, ActionDraw:
		cl.optionBtn1.SetLabel(string(ActionNewGamePrompt))
		cl.optionBtn2.SetLabel(string(ActionExit))

	case ActionExit:
		//cl.Out <- MessageGameAction{Action: ActionExit}
		//cl.App.SetRoot(cl.MenuLayout, true)
		cl.Disconnect()

	default:
		log.Println("Unknown action")
	}
	go cl.App.Draw()
}

func (cl *Client) InitGUI() {
	// Game Layout

	cl.optionBtn1 = tview.NewButton(string(ActionDrawPrompt))
	cl.optionBtn2 = tview.NewButton(string(ActionResignPrompt))
	cl.optionBtn1.SetSelectedFunc(func() {
		switch cl.optionBtn1.GetLabel() {
		case string(ActionDrawPrompt):
			go cl.HandleAction(ActionDrawPrompt)
		case string(ActionResignYes):
			go cl.HandleAction(ActionResignYes)
		case string(ActionDrawAccept):
			go cl.HandleAction(ActionDrawAccept)
		case string(ActionNewGamePrompt):
			go cl.HandleAction(ActionNewGamePrompt)
		case string(ActionNewGameAccept):
			go cl.HandleAction(ActionNewGameAccept)
		}
	})

	cl.optionBtn2.SetSelectedFunc(func() {
		switch cl.optionBtn2.GetLabel() {
		case string(ActionResignPrompt):
			go cl.HandleAction(ActionResignPrompt)
		case string(ActionResignNo):
			go cl.HandleAction(ActionResignNo)
		case string(ActionDrawReject):
			go cl.HandleAction(ActionDrawReject)
		case string(ActionExit):
			go cl.HandleAction(ActionExit)
		case string(ActionNewGameReject):
			go cl.HandleAction(ActionNewGameReject)
		}
	})

	StatusTextView = tview.NewTextView().
		SetDynamicColors(true)

	gameOptions := tview.NewGrid().
		SetColumns(3, 11, 1, 11, 3).
		SetRows(3, 1, 3, -1).
		AddItem(StatusTextView, 0, 0, 1, 5, 0, 0, false).
		AddItem(cl.optionBtn1, 2, 1, 1, 1, 0, 0, false).
		AddItem(cl.optionBtn2, 2, 3, 1, 1, 0, 0, false)

	messageInput := tview.NewInputField()
	messageInput.SetLabel("[red]>[red] ").
		SetDoneFunc(func(key tcell.Key) {
			cl.Out <- MessageGameChat{Message: messageInput.GetText(), Time: time.Now()}
			messageInput.SetText("")
		})

	ChatTextView = tview.NewTextView().
		SetScrollable(true).
		SetDynamicColors(true).
		SetWordWrap(true)

	chatGrid := tview.NewGrid().
		SetColumns(60).
		SetRows(9, 1, 1).
		AddItem(ChatTextView, 0, 0, 1, 1, 0, 0, false).
		AddItem(messageInput, 2, 0, 1, 1, 0, 0, false)

	board := tview.NewTable()

	gameLayout := tview.NewGrid().
		SetRows(-1, 10, 11, -1).
		SetColumns(-1, 30, 30, -1).
		AddItem(board, 1, 1, 1, 1, 0, 0, true).
		AddItem(gameOptions, 1, 2, 1, 1, 0, 0, false).
		AddItem(chatGrid, 2, 1, 1, 2, 0, 0, false)

	cl.Board = board
	cl.GameLayout = gameLayout
	cl.initBoard()

	// Menu Layout
	menuInput := tview.NewInputField()
	menuInput.SetLabel("[red]>[red] ").
		SetDoneFunc(func(key tcell.Key) {
			//cl.Out <- MessageGameChat{Message: messageInput.GetText(), Time: time.Now()}
			command := strings.TrimSpace(strings.ToLower(menuInput.GetText()))
			commands := strings.Split(command, " ")
			menuInput.SetText("")
			switch commands[0] {
			case "ls":
				cl.Out <- MessageGameCommand{Command: CommandLs}

			case "join":
				var roomName string
				if len(commands) > 1 {
					roomName = strings.Join(commands[1:], "_")
				}

				cl.Out <- MessageGameCommand{Command: CommandJoin, Argument: roomName}

			case "create":
				var roomName string
				if len(commands) > 1 {
					roomName = strings.Join(commands[1:], "_")
				}
				cl.Out <- MessageGameCommand{Command: CommandCreate, Argument: roomName}

			case "callme":
				var name string
				if len(commands) > 1 {
					name = strings.Join(commands[1:], "_")
					cl.Out <- MessageGameCommand{Command: CommandCallme, Argument: name}
				} else {
					currentText := MenuTextView.GetText(false)
					MenuTextView.
						SetText(fmt.Sprintf("%s\n%s", currentText, "Please provide your name after [green]callme[white] command")).
						ScrollToEnd()
				}

			case "exit":
				cl.Disconnect()

			case "about":
				currentText := MenuTextView.GetText(false)
				aboutText := `[green]Github[white]  : github.com/qnkhuat
[green]Website[white] : ngockhuat.me
[green]Twitter[white] : @qnkhuat
[green]Email[white]   : qn.khuat@gmail.com
Give gochess a star if you like it! [green]github.com/qnkhuat/chessterm[white]
				`
				MenuTextView.
					SetText(fmt.Sprintf("%s\n%s", currentText, aboutText)).
					ScrollToEnd()

			case "help":
				currentText := MenuTextView.GetText(false)
				MenuTextView.
					SetText(fmt.Sprintf("%s\n%s", currentText, commandlist)).
					ScrollToEnd()

			default:
				currentText := MenuTextView.GetText(false)
				helpText := "Invalid command. Try help"
				MenuTextView.
					SetText(fmt.Sprintf("%s\n%s", currentText, helpText)).
					ScrollToEnd()

			}
		})

	MenuTextView = tview.NewTextView().
		SetText("WELCOME TO [green]GOCHESS.CLUB[white]" + commandlist).
		SetScrollable(true).
		SetDynamicColors(true).
		SetWordWrap(true).
		ScrollToEnd()

	menuLayout := tview.NewGrid().
		SetRows(-1, 15, 1, 1, -1).
		SetColumns(-1, 60, -1).
		AddItem(MenuTextView, 1, 1, 1, 1, 0, 0, false).
		AddItem(menuInput, 3, 1, 1, 1, 0, 0, true)

	cl.MenuLayout = menuLayout
}

func (cl *Client) initBoard() {
	cl.renderBoard()
	cl.Board.SetSelectable(true, true)
	cl.Board.Select(0, 0).SetDoneFunc(func(key tcell.Key) {
		if key == tcell.KeyEscape {
			cl.Disconnect()
		}
		if key == tcell.KeyEnter {
			cl.Board.SetSelectable(true, true)
		}
	}).SetSelectedFunc(func(row, col int) {
		sq := cl.posToSquare(row, col)

		if cl.selecting {
			if sq == cl.lastSelection { // chose the last move to deactivate
				cl.selecting = false
				cl.lastSelection = 0
				cl.Board.GetCell(row, col).SetBackgroundColor(squareToColor(sq, cl.highlights))
				delete(cl.highlights, sq)
			} else { // Chosing destination
				move := fmt.Sprintf("%s%s", cl.lastSelection.String(), sq.String())
				validMoves := cl.Game.ValidMoves()
				isValid := false
				p := cl.Game.Position().Board().Piece(cl.lastSelection)
				if p.Type() == chess.Pawn && ((move[1] == '7' && move[3] == '8') || move[1] == '2' && move[3] == '1') { // Auto promoting to Queen
					move += "q"
				}
				for _, validMove := range validMoves {
					if strings.Compare(validMove.String(), move) == 0 {
						isValid = true
					}
				}
				if !isValid {
					log.Printf("invalid moves %s", move)
					delete(cl.highlights, sq)
					delete(cl.highlights, cl.lastSelection)
					cl.selecting = false
					cl.lastSelection = 0
				} else { // success
					log.Printf("Move: %s", move)
					cl.Out <- MessageMove{Move: move, Msg: "Hi"}
					delete(cl.highlights, cl.lastSelection)
					cl.lastSelection = 0
					cl.selecting = false
				}
			}
		} else {
			cl.highlights[sq] = true
			cl.selecting = true
			cl.lastSelection = sq
		}

		cl.renderBoard() // Not need to if the we have a seperated routine to highlights
	})
}

func (cl *Client) renderBoard() {
	board := cl.Game.Position().Board()
	var (
		r, f  int
		color tcell.Color
	)
	// Step through the ranks starting with the top row
	for r = 0; r <= numrows; r++ {
		// Each column
		for f = 0; f <= numcols; f++ {
			if f == 0 && r != numrows { // draw rank square
				var rank chess.Rank
				if cl.Role == White {
					rank = chess.Rank(numrows - r - 1)
				} else {
					rank = chess.Rank(r)
				}
				cell := tview.NewTableCell(rank.String()).
					SetAlign(tview.AlignCenter).
					SetSelectable(false)
				cl.Board.SetCell(r, f, cell)
				continue
			}

			if r == numrows && f > 0 { // Draw files square
				file := chess.File(f - 1)
				cell := tview.NewTableCell(fmt.Sprintf(" %s", file.String())).
					SetAlign(tview.AlignCenter).
					SetSelectable(false)
				cl.Board.SetCell(r, f, cell)
				continue
			}

			if r == numrows && f == 0 {
				continue
			}

			// Draw the pieces

			sq := cl.posToSquare(r, f)
			p := board.Piece(sq)
			ps := fmt.Sprintf(" %s", p.String())
			color = squareToColor(sq, cl.highlights)
			cell := tview.NewTableCell(ps).
				SetAlign(tview.AlignCenter).
				SetBackgroundColor(color)
			cl.Board.SetCell(r, f, cell)
		}
	}
	cl.Board.GetCell(numrows, 0).SetSelectable(false) // The bottom left tile is not used
	go cl.App.Draw()

}

func (cl *Client) Connect(port string) {
	log.Printf("Connecting to port: %s", port)
	conn, err := net.Dial("tcp", port)

	if err != nil {
		log.Println(err)
		return
	}
	cl.Conn = conn
}

func (cl *Client) HandleWrite() {
	for command := range cl.Out {
		commandData := Encode(command)
		commandTransport := MessageTransport{MsgType: command.Type(), Data: commandData}
		b := Encode(commandTransport)
		if b[len(b)-1] != '\n' { // EOF
			b = append(b, '\n')
		}
		if cl.Conn == nil {
			return
		}
		if _, err := cl.Conn.Write(b); err != nil {
			log.Fatal(err)
		}
		log.Printf("Send a msg type :%s", command.Type())
	}
}

func (cl *Client) HandleRead() {
	defer cl.Disconnect()
	scanner := bufio.NewScanner(cl.Conn)
	var messageTransport MessageTransport
	for scanner.Scan() {
		Decode(scanner.Bytes(), &messageTransport)
		log.Printf("Received a message type: %s", messageTransport.MsgType)
		switch messageTransport.MsgType {
		case TypeMessageGame:
			var message MessageGame
			Decode(messageTransport.Data, &message)
			cl.Game = GameFromFEN(message.Fen)
			if message.IsTurn {
				StatusTextView.SetText("Your turn!")
			} else {
				StatusTextView.SetText("Opponent turn!")
			}
			cl.optionBtn1.SetLabel(ActionDrawPrompt)
			cl.optionBtn2.SetLabel(ActionResignPrompt)
			cl.renderBoard()

		case TypeMessageConnect:
			var message MessageConnect
			cl.App.SetRoot(cl.GameLayout, true)
			Decode(messageTransport.Data, &message)
			cl.Game = GameFromFEN(message.Fen)
			cl.Role = message.Role
			if message.IsTurn {
				StatusTextView.SetText("Your turn!")
			} else {
				StatusTextView.SetText("Opponent turn!")
			}
			cl.renderBoard()

		case TypeMessageGameChat:
			var message MessageGameChat
			Decode(messageTransport.Data, &message)
			currentText := ChatTextView.GetText(false)
			displayText := fmt.Sprintf("[green]%s[white]: %s", strings.Title(message.Name), message.Message)
			ChatTextView.
				SetText(fmt.Sprintf("%s%s", currentText, displayText)).
				ScrollToEnd()
			go cl.App.Draw()

		case TypeMessageGameStatus:
			var message MessageGameChat
			Decode(messageTransport.Data, &message)
			StatusTextView.SetText(message.Message)

			go cl.App.Draw()

		case TypeMessageGameAction:
			var message MessageGameAction
			Decode(messageTransport.Data, &message)
			switch message.Action {

			case ActionWin, ActionLose, ActionDraw:
				status := string(message.Action)
				if message.Message != "" {
					status = fmt.Sprintf("%s by %s", status, message.Message)
				}
				StatusTextView.SetText(status)
				cl.HandleAction(message.Action)
				go cl.App.Draw()

			case ActionDrawOffer, ActionNewGameOffer: // Opponent send draw offer
				cl.HandleAction(message.Action)

			case ActionNewGameAccept:
				cl.HandleAction(ActionDraw)

			}
		case TypeMessageGameCommand:
			var message MessageGameCommand
			Decode(messageTransport.Data, &message)
			switch message.Command {

			case CommandMessage:
				currentText := MenuTextView.GetText(false)
				MenuTextView.
					SetText(fmt.Sprintf("%s\n%s", currentText, message.Argument)).
					ScrollToEnd()
				go cl.App.Draw()

			}

		default:
			log.Printf("Received Unknown action")
		}
	}
}
func (cl *Client) posToSquare(row, col int) chess.Square {
	// A1 is square 0
	if cl.Role != Black { // decending order if is white
		row = numrows - row - 1
	}
	col = col - 1 // 1 column for the rank
	return chess.Square(row*8 + col)
}
