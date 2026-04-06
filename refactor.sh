#!/bin/bash
cd internal/ui/model

# We need to be careful. m.chat is sometimes passed around or checked for nil.
sed -i 's/m\.chat\./m.activeChat()./g' *.go
sed -i 's/m\.chat /m.activeChat() /g' *.go
sed -i 's/m\.session\./m.activeSession()./g' *.go
sed -i 's/m\.session /m.activeSession() /g' *.go
sed -i 's/m\.sessionFileReads/m.activePaneObj().SessionFileReads/g' *.go
sed -i 's/m\.sessionFiles/m.activePaneObj().SessionFiles/g' *.go
sed -i 's/m\.todoSpinner/m.activePaneObj().TodoSpinner/g' *.go
sed -i 's/m\.todoIsSpinning/m.activePaneObj().TodoIsSpinning/g' *.go
sed -i 's/m\.pillsExpanded/m.activePaneObj().PillsExpanded/g' *.go
sed -i 's/m\.focusedPillSection/m.activePaneObj().FocusedPillSection/g' *.go
sed -i 's/m\.promptQueue/m.activePaneObj().PromptQueue/g' *.go
sed -i 's/m\.pillsView/m.activePaneObj().PillsView/g' *.go

